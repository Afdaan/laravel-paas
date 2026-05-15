package workers

import (
	"context"
	"log/slog"
	"time"

	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services/domain"
	"github.com/laravel-paas/backend/internal/services/project"
	"gorm.io/gorm"
)

type DomainWorker struct {
	db             *gorm.DB
	domainService  *domain.DomainService
	projectService *project.ProjectService
}

func NewDomainWorker(db *gorm.DB, domainService *domain.DomainService, projectService *project.ProjectService) *DomainWorker {
	return &DomainWorker{
		db:             db,
		domainService:  domainService,
		projectService: projectService,
	}
}

func (w *DomainWorker) Start(ctx context.Context) {
	slog.Info("Starting Domain Verification Worker (every 5 minutes)")
	
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.verifyPendingDomains()
		}
	}
}

func (w *DomainWorker) verifyPendingDomains() {
	var domains []models.CustomDomain
	// Fetch domains that are not active yet
	err := w.db.Where("status IN (?)", []string{string(models.DomainStatusPending), string(models.DomainStatusError)}).Find(&domains).Error
	if err != nil {
		slog.Error("Failed to fetch pending domains for background verification", "error", err)
		return
	}

	if len(domains) == 0 {
		return
	}

	slog.Info("Background domain verification started", "count", len(domains))

	for _, d := range domains {
		// Load project for verification context
		project, err := w.projectService.GetProjectByID(d.ProjectID)
		if err != nil {
			continue
		}

		// Perform verification
		_, err = w.domainService.VerifyDomain(d.ID, d.ProjectID, project)
		if err == nil {
			slog.Info("Domain verified in background", "domain", d.Domain)
			
			// If verified, sync Nginx for the project
			updatedProject, err := w.projectService.GetProjectByID(d.ProjectID)
			if err == nil {
				w.projectService.SyncProjectNginx(updatedProject)
				w.projectService.RecreateProjectZeroDowntime(updatedProject)
			}
		}
	}
}
