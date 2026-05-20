package domain

import (
	"context"
	"time"

	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	PriorityCritical = 3
	PriorityHigh     = 2
	PriorityNormal   = 1
	PriorityLow      = 0
)

type ReconcileQueue interface {
	Enqueue(ctx context.Context, domainID uint, priority int, cause string) error
	Dequeue(ctx context.Context, workerID string, partition int, limit int) ([]*models.PendingReconcile, error)
	Release(ctx context.Context, id uint, nextRunAfter time.Time, incrementRetry bool) error
	Complete(ctx context.Context, id uint) error
}

type gormReconcileQueue struct {
	db *gorm.DB
}

func NewReconcileQueue(db *gorm.DB) ReconcileQueue {
	return &gormReconcileQueue{db: db}
}

func (q *gormReconcileQueue) Enqueue(ctx context.Context, domainID uint, priority int, cause string) error {
	var pr models.PendingReconcile
	err := q.db.WithContext(ctx).Where("domain_id = ?", domainID).First(&pr).Error
	if err == gorm.ErrRecordNotFound {
		// New unique task. Assign partition deterministically based on domain ID
		// to allow multiple workers to run on sharded partitions without lock contention.
		pr = models.PendingReconcile{
			DomainID:  domainID,
			Priority:  priority,
			Cause:     cause,
			RunAfter:  time.Now(),
			Partition: int(domainID % 8),
		}
		return q.db.WithContext(ctx).Create(&pr).Error
	} else if err != nil {
		return err
	}

	// De-duplicate task. Promote priority if the new trigger has higher criticality,
	// and force execution to run immediately.
	updates := map[string]interface{}{}
	if priority > pr.Priority {
		updates["priority"] = priority
	}
	updates["cause"] = cause
	updates["run_after"] = time.Now()
	return q.db.WithContext(ctx).Model(&pr).Updates(updates).Error
}

func (q *gormReconcileQueue) Dequeue(ctx context.Context, workerID string, partition int, limit int) ([]*models.PendingReconcile, error) {
	var results []*models.PendingReconcile
	// Execute dequeue in an explicit transaction block with UPDATE row locks.
	// This ensures that other replica processes scanning the queue cannot select or race
	// for the same work payload.
	err := q.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prs []models.PendingReconcile
		now := time.Now()
		// Safe self-healing recovery: If a worker dies mid-execution, we rescue and retry
		// the orphaned task after its lock lease has expired (5-minute static timeout threshold).
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("partition = ? AND run_after <= ? AND (locked_by IS NULL OR locked_at < ?)", partition, now, now.Add(-5*time.Minute)).
			Order("priority desc, run_after asc").
			Limit(limit).
			Find(&prs).Error
		if err != nil {
			return err
		}

		for i := range prs {
			prs[i].LockedBy = &workerID
			prs[i].LockedAt = &now
			tx.Save(&prs[i])
			results = append(results, &prs[i])
		}
		return nil
	})
	return results, err
}

func (q *gormReconcileQueue) Release(ctx context.Context, id uint, nextRunAfter time.Time, incrementRetry bool) error {
	updates := map[string]interface{}{
		"locked_by": nil,
		"locked_at": nil,
		"run_after": nextRunAfter,
	}
	if incrementRetry {
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	} else {
		updates["retry_count"] = 0
	}
	return q.db.WithContext(ctx).Model(&models.PendingReconcile{}).Where("id = ?", id).Updates(updates).Error
}

func (q *gormReconcileQueue) Complete(ctx context.Context, id uint) error {
	return q.db.WithContext(ctx).Where("id = ?", id).Delete(&models.PendingReconcile{}).Error
}
