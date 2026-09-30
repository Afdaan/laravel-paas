package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrProjectNotFound      = errors.New("project not found")
	ErrInvalidTransition    = errors.New("invalid deployment state transition attempted")
	ErrStaleDeploymentOwner = infrastructure.ErrStaleDeploymentOwner
)

// TransitionManager enforces globally atomic, deterministic state machine progressions
// across all deployment lifecycle stages, guaranteeing monotonic event ordering.
type TransitionManager interface {
	TransitionState(ctx context.Context, projectID uint, jobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) (*models.Project, error)
	TransitionStateIfMatch(ctx context.Context, projectID uint, expectedStatus models.DeploymentStatus, expectedJobID *string, newJobID string, nextState models.DeploymentStatus, progress int, eventType, payload string) (*models.Project, bool, error)
	RequeueDeployment(ctx context.Context, projectID uint, jobID string, message string, initialStatus ...models.ProjectStatus) (*models.Project, error)
	RequeueDeploymentIfMatch(ctx context.Context, expected *models.Project, jobID string, message string, initialStatus ...models.ProjectStatus) (*models.Project, error)
	FailQueuedDeploymentIfUnchanged(ctx context.Context, projectID uint, jobID string, message string, markProjectFailed bool, restoreStatus ...models.ProjectStatus) (bool, error)
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

		// If caller provided a specific jobID, ensure project in DB is still owned by this job.
		// If another job has superseded it (e.g. admin requeued, or newer deployment queued),
		// reject transitions from the stale job owner unless admitting a new queued job.
		if jobID != "" && nextState != models.DepStatusQueued {
			if project.DeploymentJobID != nil && *project.DeploymentJobID != "" && *project.DeploymentJobID != jobID {
				return fmt.Errorf("%w: current job '%s', caller job '%s'", ErrStaleDeploymentOwner, *project.DeploymentJobID, jobID)
			}
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

		// Track precise lifecycle boundary timestamps for operational observability.
		// enqueued_at marks admission to the queue, started_at marks worker pickup;
		// keeping them separate is what makes queue wait time measurable.
		switch {
		case nextState == models.DepStatusQueued && prevState != models.DepStatusQueued:
			updates["error_log"] = nil
			updates["health_notice"] = nil
			updates["health_notice_at"] = nil
			updates["deployment_enqueued_at"] = now
			updates["deployment_started_at"] = nil
			updates["deployment_finished_at"] = nil
			updates["deployment_heartbeat_at"] = now
		case nextState == models.DepStatusPreparing && prevState != models.DepStatusPreparing:
			if prevState != models.DepStatusQueued {
				// Never queued (direct pickup): there is no measurable queue wait.
				updates["deployment_enqueued_at"] = now
			}
			updates["deployment_started_at"] = now
			updates["deployment_finished_at"] = nil
			updates["deployment_heartbeat_at"] = now
		case nextState == models.DepStatusFailed:
			updates["deployment_finished_at"] = now
			updates["error_log"] = payload
			if project.ContainerID != nil && *project.ContainerID != "" {
				updates["status"] = models.StatusRunning
			} else {
				updates["status"] = models.StatusFailed
			}
		case models.IsTerminalDeploymentStatus(nextState):
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

// FailQueuedDeploymentIfUnchanged atomically transitions a queued deployment to failed
// if and only if the project's current deployment state is still queued and its
// deployment job ID still matches the expected jobID. If another operation has
// taken ownership (e.g. a concurrent requeue with a new job ID, or a worker that
// already progressed the deployment), this method safely no-ops and returns (false, nil),
// preventing stale compensation from overwriting a newer or active deployment.
func (m *transitionManager) FailQueuedDeploymentIfUnchanged(ctx context.Context, projectID uint, jobID string, message string, markProjectFailed bool, restoreStatus ...models.ProjectStatus) (bool, error) {
	var applied bool
	var event models.DeploymentEvent

	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
			return ErrProjectNotFound
		}

		// Ensure this operation still owns the queued deployment row.
		if project.DeploymentStatus != models.DepStatusQueued || project.DeploymentJobID == nil || *project.DeploymentJobID != jobID {
			applied = false
			return nil
		}

		now := time.Now()
		updates := map[string]interface{}{
			"deployment_status":      models.DepStatusFailed,
			"deployment_progress":    0,
			"deployment_message":     message,
			"deployment_finished_at": now,
			"updated_at":             now,
		}
		if markProjectFailed {
			if project.ContainerID == nil || *project.ContainerID == "" {
				updates["status"] = models.StatusFailed
			}
		} else if len(restoreStatus) == 2 && project.Status == restoreStatus[1] {
			updates["status"] = restoreStatus[0]
		}

		if err := tx.Model(&project).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to commit deployment failure compensation: %w", err)
		}

		var nextSeq int
		tx.Raw("SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM deployment_events WHERE project_id = ? AND job_id = ?", projectID, jobID).Scan(&nextSeq)

		workerID := fmt.Sprintf("node-%s", m.hostname)
		event = models.DeploymentEvent{
			ProjectID:      project.ID,
			JobID:          jobID,
			SequenceNumber: nextSeq,
			WorkerID:       workerID,
			StateFrom:      string(models.DepStatusQueued),
			StateTo:        string(models.DepStatusFailed),
			EventType:      "queue_failed",
			Payload:        message,
			CreatedAt:      now,
		}

		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to insert deployment audit event: %w", err)
		}

		applied = true
		return nil
	})

	if err != nil {
		return false, err
	}

	if applied && event.JobID != "" && m.redisService != nil {
		if eventJSON, err := json.Marshal(event); err == nil {
			_ = m.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
		}
	}

	return applied, nil
}

// TransitionStateIfMatch atomically checks if the project currently in the database has the expected
// deployment status and expected deployment job ID before applying the transition to nextState.
// If another operation has superseded the job or transitioned the status, it safely returns (nil, false, nil)
// without modifying the row.
func (m *transitionManager) TransitionStateIfMatch(
	ctx context.Context,
	projectID uint,
	expectedStatus models.DeploymentStatus,
	expectedJobID *string,
	newJobID string,
	nextState models.DeploymentStatus,
	progress int,
	eventType, payload string,
) (*models.Project, bool, error) {
	var updatedProject models.Project
	var event models.DeploymentEvent
	var applied bool

	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
			return ErrProjectNotFound
		}

		// Predicate check: ensure the project has not been superseded by an admin override
		// or progressed by a worker.
		if project.DeploymentStatus != expectedStatus || !sameDeploymentJob(project.DeploymentJobID, expectedJobID) {
			applied = false
			return nil
		}

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
		activeJobID := newJobID
		if activeJobID == "" && project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
			activeJobID = *project.DeploymentJobID
		}
		if newJobID != "" {
			updates["deployment_job_id"] = newJobID
		}

		switch {
		case nextState == models.DepStatusQueued && prevState != models.DepStatusQueued:
			updates["deployment_enqueued_at"] = now
			updates["deployment_started_at"] = nil
			updates["deployment_finished_at"] = nil
			updates["deployment_heartbeat_at"] = now
		case nextState == models.DepStatusPreparing && prevState != models.DepStatusPreparing:
			if prevState != models.DepStatusQueued {
				updates["deployment_enqueued_at"] = now
			}
			updates["deployment_started_at"] = now
			updates["deployment_finished_at"] = nil
			updates["deployment_heartbeat_at"] = now
		case nextState == models.DepStatusFailed:
			updates["deployment_finished_at"] = now
			updates["error_log"] = payload
			if project.ContainerID != nil && *project.ContainerID != "" {
				updates["status"] = models.StatusRunning
			} else {
				updates["status"] = models.StatusFailed
			}
		case models.IsTerminalDeploymentStatus(nextState):
			updates["deployment_finished_at"] = now
		}

		if err := tx.Model(&project).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to commit deployment status updates: %w", err)
		}

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

		if err := tx.First(&updatedProject, projectID).Error; err != nil {
			return err
		}

		applied = true
		return nil
	})

	if err != nil {
		return nil, false, err
	}

	if applied && event.JobID != "" && m.redisService != nil {
		if eventJSON, err := json.Marshal(event); err == nil {
			_ = m.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
		}
	}

	if !applied {
		return nil, false, nil
	}

	return &updatedProject, true, nil
}

// RequeueDeployment atomically transitions both deployment status and overall project status
// to queued with a new job ID, resets runtime progression metadata, and records an audit event,
// all executed within a single database transaction under a row-level lock.
func (m *transitionManager) RequeueDeployment(ctx context.Context, projectID uint, jobID string, message string, initialStatus ...models.ProjectStatus) (*models.Project, error) {
	return m.requeueDeployment(ctx, projectID, nil, jobID, message, initialStatus...)
}

func (m *transitionManager) RequeueDeploymentIfMatch(ctx context.Context, expected *models.Project, jobID string, message string, initialStatus ...models.ProjectStatus) (*models.Project, error) {
	if expected == nil || expected.ID == 0 {
		return nil, fmt.Errorf("expected project is required")
	}
	return m.requeueDeployment(ctx, expected.ID, expected, jobID, message, initialStatus...)
}

func (m *transitionManager) requeueDeployment(ctx context.Context, projectID uint, expected *models.Project, jobID string, message string, initialStatus ...models.ProjectStatus) (*models.Project, error) {
	var updatedProject models.Project
	var event models.DeploymentEvent

	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var project models.Project
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, projectID).Error; err != nil {
			return ErrProjectNotFound
		}
		if expected != nil && (project.DeploymentStatus != expected.DeploymentStatus || !sameDeploymentJob(project.DeploymentJobID, expected.DeploymentJobID) || project.Status != expected.Status) {
			return ErrStaleDeploymentOwner
		}

		targetStatus := models.StatusQueued
		if project.ContainerID != nil && *project.ContainerID != "" && (project.Status == models.StatusRunning || project.Status == models.StatusStopped) {
			targetStatus = project.Status
		}
		if len(initialStatus) > 0 && initialStatus[0] != "" {
			targetStatus = initialStatus[0]
		}

		now := time.Now()
		updates := map[string]interface{}{
			"deployment_status":       models.DepStatusQueued,
			"status":                  targetStatus,
			"deployment_job_id":       jobID,
			"deployment_message":      message,
			"deployment_progress":     0,
			"error_log":               nil,
			"health_notice":           nil,
			"health_notice_at":        nil,
			"deployment_enqueued_at":  now,
			"deployment_started_at":   nil,
			"deployment_finished_at":  nil,
			"deployment_heartbeat_at": now,
			"updated_at":              now,
		}

		if err := tx.Model(&project).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to commit requeue deployment updates: %w", err)
		}

		var nextSeq int
		tx.Raw("SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM deployment_events WHERE project_id = ? AND job_id = ?", projectID, jobID).Scan(&nextSeq)

		workerID := fmt.Sprintf("node-%s", m.hostname)
		event = models.DeploymentEvent{
			ProjectID:      project.ID,
			JobID:          jobID,
			SequenceNumber: nextSeq,
			WorkerID:       workerID,
			StateFrom:      string(project.DeploymentStatus),
			StateTo:        string(models.DepStatusQueued),
			EventType:      "admin_manual_requeue",
			Payload:        message,
			CreatedAt:      now,
		}

		if err := tx.Create(&event).Error; err != nil {
			return fmt.Errorf("failed to insert deployment audit event: %w", err)
		}

		return tx.First(&updatedProject, projectID).Error
	})

	if err != nil {
		return nil, err
	}

	if event.JobID != "" && m.redisService != nil {
		if eventJSON, err := json.Marshal(event); err == nil {
			_ = m.redisService.PublishDeploymentEvent(projectID, string(eventJSON))
		}
	}

	return &updatedProject, nil
}

func sameDeploymentJob(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
