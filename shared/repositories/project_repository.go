// ===========================================
// Project Repository
// ===========================================
// Handles data persistence for Project model
// ===========================================
package repositories

import (
	"errors"
	"strings"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"
)

type ProjectRepository interface {
	GetByID(id uint) (*models.Project, error)
	GetByIDForNginx(id uint) (*models.Project, error)
	GetByUID(uid string) (*models.Project, error)
	GetBySubdomain(subdomain string) (*models.Project, error)
	List(page, limit int, userID uint, status string, search string) ([]models.Project, int64, error)
	ListByUserID(userID uint) ([]models.Project, error)
	ListAll() ([]models.Project, error)
	ListExpired() ([]models.Project, error)
	ListByStatus(status models.ProjectStatus) ([]models.Project, error)
	ListByStatuses(statuses []models.ProjectStatus) ([]models.Project, error)
	Create(project *models.Project) error
	Update(project *models.Project) error
	UpdateStatus(id uint, status models.ProjectStatus) error
	UpdateMetadata(id uint, updates map[string]interface{}) error
	UpdateConfigHash(id uint, newHash string, expectedOldHash string) error
	Delete(id uint) error
	UpdateActivity(id uint) error
	CountTotal() (int64, error)
	CountByUserID(userID uint) (int64, error)
	CountRunning() (int64, error)
	GetRunningWithContainers() ([]models.Project, error)
	RecordDeploymentEvent(event *models.DeploymentEvent) error
	ListDeploymentEventsByProjectID(projectID uint) ([]models.DeploymentEvent, error)
	ListAllDeploymentEventsByProjectID(projectID uint) ([]models.DeploymentEvent, error)
	ListByDeploymentStatuses(statuses []models.DeploymentStatus) ([]models.Project, error)
	UpdateDeploymentStatus(id uint, status models.DeploymentStatus, message string, progress int, jobID string) error
	UpdateDeploymentHeartbeat(id uint) error
	PromoteRolloutContainer(id uint, newContainerID string) error
	ResolveInstallationID(userID uint, owner string) (int64, error)
	VerifyInstallationID(installationID int64, owner string) (bool, error)
	SaveDatabaseInstance(instance *models.DatabaseInstance) error
}

type projectRepository struct {
	db *gorm.DB
}

func NewProjectRepository(db *gorm.DB) ProjectRepository {
	return &projectRepository{db: db}
}

func (r *projectRepository) GetByID(id uint) (*models.Project, error) {
	var project models.Project
	if err := r.db.Preload("User").Preload("CustomDomains").Preload("DatabaseInstance").First(&project, id).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *projectRepository) GetByIDForNginx(id uint) (*models.Project, error) {
	var project models.Project
	verifiedStatuses := []models.CustomDomainStatus{
		models.DomainStatusDNSVerified,
		models.DomainStatusSSLQueued,
		models.DomainStatusSSLProvisioning,
		models.DomainStatusSSLActive,
		models.DomainStatusActive,
		models.DomainStatusPropagationPending,
		models.DomainStatusDegraded,
		models.DomainStatusRenewalPending,
		models.DomainStatusRenewalFailed,
	}
	err := r.db.
		Preload("User").
		Preload("CustomDomains", "status IN ?", verifiedStatuses).
		First(&project, id).Error
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *projectRepository) GetByUID(uid string) (*models.Project, error) {
	var project models.Project
	if err := r.db.Preload("User").Preload("CustomDomains").Preload("DatabaseInstance").Where("uid = ?", uid).First(&project).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *projectRepository) GetBySubdomain(subdomain string) (*models.Project, error) {
	var project models.Project
	if err := r.db.Preload("CustomDomains").Where("subdomain = ?", subdomain).First(&project).Error; err != nil {
		return nil, err
	}
	return &project, nil
}

func (r *projectRepository) List(page, limit int, userID uint, status string, search string) ([]models.Project, int64, error) {
	var projects []models.Project
	var total int64

	query := r.db.Model(&models.Project{})

	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if status != "" && status != "all" {
		query = query.Where("status = ?", status)
	}
	if search != "" {
		query = query.Where("name LIKE ? OR subdomain LIKE ?", "%"+search+"%", "%"+search+"%")
	}

	query.Count(&total)

	offset := (page - 1) * limit
	err := query.Preload("User").Preload("CustomDomains").Preload("DatabaseInstance").Order("created_at DESC").Offset(offset).Limit(limit).Find(&projects).Error

	return projects, total, err
}

func (r *projectRepository) ListByUserID(userID uint) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("CustomDomains").Where("user_id = ?", userID).Order("created_at DESC").Find(&projects).Error
	return projects, err
}

func (r *projectRepository) ListAll() ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("User").Preload("CustomDomains").Preload("DatabaseInstance").Order("created_at DESC").Find(&projects).Error
	return projects, err
}

func (r *projectRepository) ListExpired() ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Where("expires_at IS NOT NULL AND expires_at < NOW() AND status != ?", models.StatusStopped).Find(&projects).Error
	return projects, err
}

func (r *projectRepository) ListByStatus(status models.ProjectStatus) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("CustomDomains").Where("status = ?", status).Find(&projects).Error
	return projects, err
}

func (r *projectRepository) ListByStatuses(statuses []models.ProjectStatus) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("CustomDomains").Where("status IN ?", statuses).Find(&projects).Error
	return projects, err
}

func (r *projectRepository) ListByDeploymentStatuses(statuses []models.DeploymentStatus) ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Preload("CustomDomains").Where("deployment_status IN ?", statuses).Find(&projects).Error
	return projects, err
}

func (r *projectRepository) Create(project *models.Project) error {
	return r.db.Create(project).Error
}

func (r *projectRepository) Update(project *models.Project) error {
	return r.db.Omit("User").Save(project).Error
}

func (r *projectRepository) UpdateStatus(id uint, status models.ProjectStatus) error {
	return r.db.Model(&models.Project{}).Where("id = ?", id).Update("status", status).Error
}

func (r *projectRepository) UpdateMetadata(id uint, updates map[string]interface{}) error {
	return r.db.Model(&models.Project{}).Where("id = ?", id).Updates(updates).Error
}

func (r *projectRepository) UpdateConfigHash(id uint, newHash string, expectedOldHash string) error {
	query := r.db.Model(&models.Project{}).Where("id = ?", id)
	if expectedOldHash != "" {
		query = query.Where("config_hash = ? OR config_hash IS NULL OR config_hash = ''", expectedOldHash)
	}
	res := query.Update("config_hash", newHash)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("conditional config_hash update failed: hash was modified concurrently")
	}
	return nil
}

func (r *projectRepository) Delete(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete associated custom domains
		if err := tx.Unscoped().Where("project_id = ?", id).Delete(&models.CustomDomain{}).Error; err != nil {
			return err
		}
		
		// Delete associated database backups
		if err := tx.Unscoped().Where("project_id = ?", id).Delete(&models.DatabaseBackup{}).Error; err != nil {
			return err
		}
		
		// Delete associated database instances
		if err := tx.Unscoped().Where("project_id = ?", id).Delete(&models.DatabaseInstance{}).Error; err != nil {
			return err
		}
		
		// Delete associated deployment events
		if err := tx.Unscoped().Where("project_id = ?", id).Delete(&models.DeploymentEvent{}).Error; err != nil {
			return err
		}

		// Delete associated resource logs
		if err := tx.Unscoped().Where("project_id = ?", id).Delete(&models.ResourceLog{}).Error; err != nil {
			return err
		}

		// Delete the project itself
		if err := tx.Unscoped().Delete(&models.Project{}, id).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *projectRepository) UpdateActivity(id uint) error {
	return r.db.Model(&models.Project{}).Where("id = ?", id).Update("last_accessed_at", gorm.Expr("NOW()")).Error
}

func (r *projectRepository) CountTotal() (int64, error) {
	var count int64
	err := r.db.Model(&models.Project{}).Count(&count).Error
	return count, err
}

func (r *projectRepository) CountByUserID(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Project{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}

func (r *projectRepository) CountRunning() (int64, error) {
	var count int64
	err := r.db.Model(&models.Project{}).Where("status = ?", models.StatusRunning).Count(&count).Error
	return count, err
}

func (r *projectRepository) GetRunningWithContainers() ([]models.Project, error) {
	var projects []models.Project
	err := r.db.Where("status = ? AND container_id IS NOT NULL", models.StatusRunning).Find(&projects).Error
	return projects, err
}

func (r *projectRepository) RecordDeploymentEvent(event *models.DeploymentEvent) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var dummy uint
		tx.Raw("SELECT id FROM projects WHERE id = ? FOR UPDATE", event.ProjectID).Scan(&dummy)
		var nextSeq int
		tx.Raw("SELECT COALESCE(MAX(sequence_number), 0) + 1 FROM deployment_events WHERE project_id = ? AND job_id = ?", event.ProjectID, event.JobID).Scan(&nextSeq)
		event.SequenceNumber = nextSeq
		return tx.Create(event).Error
	})
}

func (r *projectRepository) ListDeploymentEventsByProjectID(projectID uint) ([]models.DeploymentEvent, error) {
	var project models.Project
	if err := r.db.Select("deployment_job_id").First(&project, projectID).Error; err != nil {
		return nil, err
	}
	var events []models.DeploymentEvent
	query := r.db.Where("project_id = ? AND event_type NOT IN (?, ?, ?)", projectID, "lease_acquired", "lease_renewed", "lease_released")
	if project.DeploymentJobID != nil && *project.DeploymentJobID != "" {
		query = query.Where("job_id = ?", *project.DeploymentJobID)
	}
	err := query.Order("sequence_number ASC, created_at ASC").Find(&events).Error
	return events, err
}

func (r *projectRepository) ListAllDeploymentEventsByProjectID(projectID uint) ([]models.DeploymentEvent, error) {
	var events []models.DeploymentEvent
	err := r.db.Where("project_id = ? AND event_type NOT IN (?, ?, ?)", projectID, "lease_acquired", "lease_renewed", "lease_released").
		Order("created_at DESC, sequence_number DESC").Limit(50).Find(&events).Error
	return events, err
}

func (r *projectRepository) UpdateDeploymentStatus(id uint, status models.DeploymentStatus, message string, progress int, jobID string) error {
	updates := map[string]interface{}{
		"deployment_status":       status,
		"deployment_message":      message,
		"deployment_progress":     progress,
		"deployment_heartbeat_at": gorm.Expr("NOW()"),
	}
	if jobID != "" {
		updates["deployment_job_id"] = jobID
	}
	if status == models.DepStatusPreparing || status == models.DepStatusQueued {
		updates["deployment_started_at"] = gorm.Expr("NOW()")
	}
	if status == models.DepStatusCompleted || status == models.DepStatusFailed || status == models.DepStatusRollback || status == models.DepStatusCancelled {
		updates["deployment_finished_at"] = gorm.Expr("NOW()")
	}
	return r.db.Model(&models.Project{}).Where("id = ?", id).Updates(updates).Error
}

func (r *projectRepository) PromoteRolloutContainer(id uint, newContainerID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"container_id":         &newContainerID,
			"rollout_container_id": nil,
			"status":               models.StatusRunning,
		}
		return tx.Model(&models.Project{}).Where("id = ?", id).Updates(updates).Error
	})
}

// UpdateDeploymentHeartbeat updates the deployment lease timestamp in the database.
// Executing this query independently isolates the heartbeat loop from concurrent mutations on the project model.
func (r *projectRepository) UpdateDeploymentHeartbeat(id uint) error {
	return r.db.Model(&models.Project{}).Where("id = ?", id).Update("deployment_heartbeat_at", gorm.Expr("NOW()")).Error
}

func (r *projectRepository) ResolveInstallationID(userID uint, owner string) (int64, error) {
	var inst models.GithubAppInstallation
	if owner != "" {
		// 1. Try to find the installation belonging to the repository owner for this specific user
		err := r.db.Where("user_id = ? AND LOWER(account_name) = ?", userID, strings.ToLower(owner)).First(&inst).Error
		if err == nil && inst.InstallationID != 0 {
			return inst.InstallationID, nil
		}
	}
	// 2. Fallback to the sole installation if they only have one
	var count int64
	r.db.Model(&models.GithubAppInstallation{}).Where("user_id = ?", userID).Count(&count)
	if count == 1 {
		err := r.db.Where("user_id = ?", userID).First(&inst).Error
		if err == nil && inst.InstallationID != 0 {
			if owner == "" || strings.EqualFold(inst.AccountName, owner) {
				return inst.InstallationID, nil
			}
		}
	}
	return 0, errors.New("installation not found")
}

func (r *projectRepository) VerifyInstallationID(installationID int64, owner string) (bool, error) {
	var count int64
	err := r.db.Model(&models.GithubAppInstallation{}).
		Where("installation_id = ? AND LOWER(account_name) = ?", installationID, strings.ToLower(owner)).
		Count(&count).Error
	return count > 0, err
}

func (r *projectRepository) SaveDatabaseInstance(instance *models.DatabaseInstance) error {
	return r.db.Save(instance).Error
}
