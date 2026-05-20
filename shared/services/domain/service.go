package domain

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/infrastructure/nginx"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/repositories"
	"github.com/laravel-paas/shared/repository"
	"gorm.io/gorm"
)

type NginxReloader interface {
	SyncProjectNginxFrom(project *models.Project, triggerSource string) (string, error)
	GetSSLStatus(domain string) (*nginx.SSLStatusResponse, error)
}

// PollerState tracks starting time of each domain's SSL status poller
type PollerState struct {
	StartedAt time.Time
}

// DomainService handles custom domain management and verification, wrapping new decoupled orchestration layers
type DomainService struct {
	cfg            *config.Config
	db             *gorm.DB
	redisService   *infrastructure.RedisService
	projectService NginxReloader
	projectRepo    repositories.ProjectRepository
	activePollers  sync.Map
	wg             sync.WaitGroup

	// Decoupled Orchestration Layers
	repo         repository.DomainRepository
	stateMachine *DomainStateMachine
	reconciler   *Reconciler
	queue        ReconcileQueue
}

func NewDomainService(cfg *config.Config, db *gorm.DB, redisService *infrastructure.RedisService, projectService NginxReloader, projectRepo repositories.ProjectRepository) *DomainService {
	repo := repository.NewDomainRepository(db)
	outbox := NewOutboxService(db, redisService)
	audit := NewAuditService(db)
	stateMachine := NewDomainStateMachine(repo, outbox, audit)
	leaseProvider := NewRedisLeaseProvider(redisService)
	queue := NewReconcileQueue(db)
	reconciler := NewReconciler(repo, queue, leaseProvider, stateMachine)

	return &DomainService{
		cfg:            cfg,
		db:             db,
		redisService:   redisService,
		projectService: projectService,
		projectRepo:    projectRepo,
		repo:           repo,
		stateMachine:   stateMachine,
		reconciler:     reconciler,
		queue:          queue,
	}
}

// GetDomainByID retrieves a custom domain by its ID
func (s *DomainService) GetDomainByID(domainID uint) (*models.CustomDomain, error) {
	return s.repo.GetByID(context.Background(), domainID)
}

// RecordEvent appends a lifecycle transition event (durable transactional outbox fallback)
func (s *DomainService) RecordEvent(domain *models.CustomDomain, stateFrom, stateTo models.CustomDomainStatus, eventType, payload, errMsg string) error {
	event, err := s.RecordEventTx(s.db, domain, stateFrom, stateTo, eventType, payload, errMsg)
	if err != nil {
		return err
	}
	eventBytes, _ := json.Marshal(event)
	_ = s.redisService.PublishDomainEvent(domain.ID, domain.ProjectID, string(eventBytes))
	return nil
}

// RecordEventTx executes event persistence inside an active database transaction.
func (s *DomainService) RecordEventTx(tx *gorm.DB, domain *models.CustomDomain, stateFrom, stateTo models.CustomDomainStatus, eventType, payload, errMsg string) (*models.DomainEvent, error) {
	var nextSeq int
	err := tx.Raw("UPDATE custom_domains SET current_sequence = COALESCE(current_sequence, 0) + 1 WHERE id = ? RETURNING current_sequence", domain.ID).Scan(&nextSeq).Error
	if err != nil {
		return nil, fmt.Errorf("failed atomic sequence increment: %w", err)
	}
	domain.CurrentSequence = nextSeq

	event := &models.DomainEvent{
		DomainID:       domain.ID,
		JobID:          fmt.Sprintf("job-%d-%d", domain.ID, time.Now().UnixNano()),
		SequenceNumber: domain.CurrentSequence,
		StateFrom:      stateFrom,
		StateTo:        stateTo,
		EventType:      eventType,
		Payload:        payload,
		Error:          errMsg,
		EventVersion:   1,
		CreatedAt:      time.Now(),
	}

	if err := tx.Create(event).Error; err != nil {
		slog.Error("Failed to persist DomainEvent inside tx", "domainID", domain.ID, "error", err)
		return nil, err
	}

	return event, nil
}

// TransitionState deterministically mutates domain status via DomainStateMachine.
// Centralizing state mutations through the StateMachine guarantees that business aggregates,
// transaction locking, outbox streaming, and audit trails are always atomically executed.
func (s *DomainService) TransitionState(domain *models.CustomDomain, nextState models.CustomDomainStatus, errCode models.DomainErrorCode, errMsg string) error {
	ctx := context.Background()
	err := s.stateMachine.Transition(ctx, domain.ID, nextState, string(errCode))
	if err != nil {
		return err
	}

	// Update local struct state to match state machine results for backward compatibility.
	domain.Status = nextState
	domain.ErrorCode = errCode
	domain.ErrorMessage = errMsg
	return nil
}

// TransitionStateCtx deterministically mutates domain status with context safety.
// It explicitly inspects context state first to block actions if context is cancelled,
// preventing stale actions from updating the state machine after lease or signal loss.
func (s *DomainService) TransitionStateCtx(ctx context.Context, domain *models.CustomDomain, nextState models.CustomDomainStatus, errCode models.DomainErrorCode, errMsg string) error {
	if ctx != nil && ctx.Err() != nil {
		slog.Warn("Operation aborted due to context cancellation or lock lease loss", "domainID", domain.ID, "nextState", nextState, "error", ctx.Err())
		return fmt.Errorf("operation aborted due to context cancellation or lock lease loss: %w", ctx.Err())
	}
	return s.TransitionState(domain, nextState, errCode, errMsg)
}

// SubscribeEvents subscribes to domain realtime lifecycle audit events.
func (s *DomainService) SubscribeEvents(ctx context.Context, domainID uint) (<-chan string, error) {
	return s.redisService.SubscribeDomainEvents(ctx, domainID)
}

// SubscribeProjectEvents subscribes to project-wide realtime lifecycle audit events.
func (s *DomainService) SubscribeProjectEvents(ctx context.Context, projectID uint) (<-chan string, error) {
	return s.redisService.SubscribeProjectEvents(ctx, projectID)
}

// ListEventsAfterSequence fetches missed events after a specific sequence number.
func (s *DomainService) ListEventsAfterSequence(domainID uint, afterSeq int) ([]models.DomainEvent, error) {
	var events []models.DomainEvent
	err := s.db.Where("domain_id = ? AND sequence_number > ?", domainID, afterSeq).Order("sequence_number ASC").Limit(100).Find(&events).Error
	return events, err
}

// ListProjectEventsAfterSequence fetches missed project-wide events.
func (s *DomainService) ListProjectEventsAfterSequence(projectID uint, afterSeq int) ([]models.DomainEvent, error) {
	var events []models.DomainEvent
	subQuery := s.db.Model(&models.CustomDomain{}).Select("id").Where("project_id = ?", projectID)
	err := s.db.Where("domain_id IN (?) AND sequence_number > ?", subQuery, afterSeq).Order("sequence_number ASC").Limit(200).Find(&events).Error
	return events, err
}

// StartLockHeartbeat starts a background watchdog goroutine extending lock TTL.
func (s *DomainService) StartLockHeartbeat(ctx context.Context, cancel context.CancelFunc, domain *models.CustomDomain, token string, ttl time.Duration) {
	ticker := time.NewTicker(ttl / 2)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.redisService.RenewDomainLock(domain.ID, token, ttl); err != nil {
					metrics.GetCollector().IncrLeaseRenewalFailures()
					metrics.GetCollector().IncrLeaseOwnershipLoss()
					slog.Error("Domain lock heartbeat renewal failed, cancelling operation context", "domainID", domain.ID, "token", token, "error", err)
					eventJSON := fmt.Sprintf(`{"event_type":"lease_loss","domain_id":%d,"error":"Worker lost lock lease ownership"}`, domain.ID)
					_ = s.redisService.PublishDomainEvent(domain.ID, domain.ProjectID, eventJSON)
					cancel()
					return
				}
			}
		}
	}()
}

// GetMetrics retrieves operational metrics.
func (s *DomainService) GetMetrics() (map[string]interface{}, error) {
	return s.redisService.GetDomainMetrics()
}

// SafeGo starts a goroutine under a recovery barrier.
func (s *DomainService) SafeGo(ctx context.Context, domainID, projectID uint, operation string, fn func(ctx context.Context) error) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				metrics.GetCollector().IncrDomainPanicRecovery()
				buf := make([]byte, 2048)
				n := runtime.Stack(buf, false)
				slog.Error("Panic recovered in domain SafeGo",
					"domainID", domainID,
					"projectID", projectID,
					"operation", operation,
					"recover", r,
					"stack", string(buf[:n]),
				)
			}
		}()

		if err := fn(ctx); err != nil {
			slog.Error("Domain async operation failed",
				"domainID", domainID,
				"projectID", projectID,
				"operation", operation,
				"error", err,
			)
		}
	}()
}

// Shutdown blocks until background operations complete.
func (s *DomainService) Shutdown() {
	slog.Info("Waiting for DomainService background operations to complete...")
	s.wg.Wait()
	slog.Info("DomainService shutdown complete.")
}
