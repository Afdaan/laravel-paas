package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrProjectNotFound   = errors.New("project not found")
	ErrInvalidTransition = errors.New("invalid deployment state transition attempted")
)

// TransitionManager enforces globally atomic, deterministic state machine progressions
// across all deployment lifecycle stages, guaranteeing monotonic event ordering.
type TransitionManager interface {
	TransitionState(ctx context.Context, projectID uint, jobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) (*models.Project, error)
}

type transitionManager struct {
	db           *gorm.DB
	redisService *infrastructure.RedisService
	hostname     string
}

func NewTransitionManager(db *gorm.DB, redisService *infrastructure.RedisService) TransitionManager {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "orchestrator-node"
	}
	return &transitionManager{
		db:           db,
		redisService: redisService,
		hostname:     hostname,
	}
}

// TransitionState atomically verifies the transition validity against the global state machine,
// updates the project deployment execution status, generates a monotonically ordered audit event,
// and broadcasts the event over Redis Pub/Sub.
func (m *transitionManager) TransitionState(ctx context.Context, projectID uint, jobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) (*models.Project, error) {
	var updatedProject models.Project
	var event models.DeploymentEvent

	// Execute within an atomic database transaction to prevent split-brain state inconsistencies
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
			return ErrProjectNotFound
		}

		// Enforce state machine progression rules. Terminal, rollback, or interrupt transitions
		// remain valid globally to ensure robust operational recovery during watchdog evictions.
		if !models.IsValidDeploymentTransition(project.DeploymentStatus, nextState) {
			return fmt.Errorf("%w: from '%s' to '%s'", ErrInvalidTransition, project.DeploymentStatus, nextState)
		}

		prevState := project.DeploymentStatus
		now := time.Now()

		updates := map[string]interface{}{
			"deployment_status":   nextState,
			"deployment_progress": progress,
			"deployment_message":  payload,
			"updated_at":          now,
		}
		activeJobID := jobID
		if activeJobID == "" && project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
			activeJobID = *project.DeploymentJobID
		}
		if jobID != "" {
			updates["deployment_job_id"] = jobID
		}

		// Track precise lifecycle boundary timestamps for operational observability
		if nextState == models.DepStatusQueued || nextState == models.DepStatusPreparing {
			updates["deployment_started_at"] = now
			updates["deployment_finished_at"] = nil
		} else if nextState == models.DepStatusCompleted || nextState == models.DepStatusFailed || nextState == models.DepStatusCancelled {
			updates["deployment_finished_at"] = now
		}

		if err := tx.Model(&project).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to commit deployment status updates: %w", err)
		}

		// Query monotonic sequence number dynamically within the atomic lock
		var nextSeq int
		tx.Raw("SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM deployment_events WHERE project_id = ? AND job_id = ?", projectID, activeJobID).Scan(&nextSeq)

		workerID := fmt.Sprintf("node-%s", m.hostname)
		event = models.DeploymentEvent{
			ProjectID:      project.ID,
			JobID:          activeJobID,
			SequenceNumber: nextSeq,
			WorkerID:       workerID,
			StateFrom:      string(prevState),
			StateTo:        string(nextState),
			EventType:      eventType,
			Payload:        payload,
			CreatedAt:      now,
		}

		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to insert deployment audit event: %w", err)
		}

		updatedProject = project
		updatedProject.DeploymentStatus = nextState
		updatedProject.DeploymentProgress = progress
		msg := payload
		updatedProject.DeploymentMessage = &msg
		if jobID != "" {
			jID := jobID
			updatedProject.DeploymentJobID = &jID
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Broadcast successful transition event asynchronously
	if eventJSON, err := json.Marshal(event); err == nil && m.redisService != nil {
		_ = m.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
	}

	return &updatedProject, nil
}
