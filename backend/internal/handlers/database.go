// ===========================================
// Database Manager Handler
// ===========================================
// Allows users to manage their project databases
// ===========================================
package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	"github.com/laravel-paas/backend/internal/services/billing"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"github.com/laravel-paas/shared/services/billinggate"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxDatabaseImportBytes      = 5 * 1024 * 1024
	maxDatabaseImportStatements = 200
)

// DatabaseHandler handles database management endpoints
type DatabaseHandler struct {
	db                 *gorm.DB
	cfg                *config.Config
	databaseService    *services.DatabaseService
	projectService     *projectServicePkg.ProjectService
	redisService       *infrastructure.RedisService
	secretStoreService *services.SecretStoreService
	localIPs           []string
	ipsOnce            sync.Once
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(
	db *gorm.DB,
	cfg *config.Config,
	databaseService *services.DatabaseService,
	projectService *projectServicePkg.ProjectService,
	redisService *infrastructure.RedisService,
	secretStoreService *services.SecretStoreService,
) *DatabaseHandler {
	return &DatabaseHandler{
		db:                 db,
		cfg:                cfg,
		databaseService:    databaseService,
		projectService:     projectService,
		redisService:       redisService,
		secretStoreService: secretStoreService,
	}
}

// getProjectForUser fetches project and validates ownership
func (h *DatabaseHandler) getProjectForUser(c *fiber.Ctx) (*models.Project, error) {
	idParam := c.Params("id")

	// Query project strictly by string UID.
	project, err := h.projectService.GetProjectByUID(idParam)
	if err != nil || project == nil {
		return nil, fmt.Errorf("project not found")
	}

	uidVal := c.Locals("user_id")
	roleVal := c.Locals("role")

	if uidVal == nil || roleVal == nil {
		return nil, fmt.Errorf("unauthorized: missing user context")
	}

	userID, okUID := uidVal.(uint)
	roleStr, okRole := roleVal.(string)

	if !okUID || !okRole {
		return nil, fmt.Errorf("internal server error: invalid user context format")
	}

	role := models.Role(roleStr)

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && project.UserID != userID {
		return nil, fmt.Errorf("project not found")
	}

	return project, nil
}

func (h *DatabaseHandler) requireProjectWithDatabase(c *fiber.Ctx) (*models.Project, error) {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return nil, c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	if project.DatabaseName == nil || *project.DatabaseName == "" {
		return nil, c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not enabled for this project"})
	}
	return project, nil
}

// recordAuditLog stores visual designer and management transitions in the platform AuditLog
func (h *DatabaseHandler) recordAuditLog(c *fiber.Ctx, projectID uint, operation, fromState, toState, status string, errMsg string) {
	if err := h.recordAuditLogRequired(c, projectID, operation, fromState, toState, status, errMsg); err != nil {
		slog.Warn("Failed to record database audit log", "project_id", projectID, "error", err)
	}
}

func (h *DatabaseHandler) recordAuditLogRequired(c *fiber.Ctx, projectID uint, operation, fromState, toState, status string, errMsg string) error {
	userID := uint(0)
	if uidVal := c.Locals("user_id"); uidVal != nil {
		if id, ok := uidVal.(uint); ok {
			userID = id
		}
	}

	log := &models.AuditLog{
		DomainID:     0,
		Operation:    operation,
		StateFrom:    fromState,
		StateTo:      toState,
		Status:       status,
		ErrorMessage: errMsg,
		TraceID:      fmt.Sprintf("ip:%s;user:%d;project:%d", c.IP(), userID, projectID),
		CreatedAt:    time.Now(),
	}

	return h.databaseService.LogAudit(log)
}

// GetCredentials returns database credentials
func (h *DatabaseHandler) GetCredentials(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err := h.recordAuditLogRequired(c, project.ID, "db_reveal_credentials", "active", "active", "completed", ""); err != nil {
		return apperr.New(500, "DATABASE_CREDENTIAL_AUDIT_FAILED", "Unable to record credential access")
	}
	if err != nil {
		// Fallback to legacy MySQL details if DatabaseInstance not initialized yet
		c.Set(fiber.HeaderCacheControl, "no-store")
		return c.JSON(fiber.Map{
			"engine":   "mysql",
			"host":     infrastructure.MySQLContainerName(),
			"port":     infrastructure.MySQLPort(),
			"database": project.GetDatabaseName(),
			"username": project.GetDatabaseName(),
			"password": project.DatabasePassword,
		})
	}

	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(fiber.Map{
		"engine":   instance.Engine,
		"host":     instance.Host,
		"port":     instance.Port,
		"database": instance.Name,
		"username": instance.Username,
		"password": instance.Password, // Never expose in GORM JSON but exposed in this secure reveal-on-demand endpoint
	})
}

// RotateCredentials generates a new random password and hot-swaps it into container
func (h *DatabaseHandler) RotateCredentials(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned for this project"})
	}

	var jobID string
	lockedProject, rotatedInstance, envSyncGeneration, err := services.NewDatabaseCredentialRotationService(h.db).StartOrResume(context.Background(), project.ID, instance.ID)
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "failed", err.Error())
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": false, "message": "Credential rotation will retry automatically"})
	}
	instance = &rotatedInstance

	// 3. Trigger fan-out env propagation to OTHER projects bound to the same secret store
	h.secretStoreService.PropagateDatabaseEnvFanout(&lockedProject)

	if jobID, err = h.redisService.EnqueueDeploymentEnvSync(lockedProject.ID, lockedProject.UserID, envSyncGeneration); err != nil {
		slog.Error("Queue durable database environment sync failed", "project_id", lockedProject.ID, "generation", envSyncGeneration, "error", err)
	} else {
		now := time.Now()
		msg := "Credentials rotation env update"
		errUpdate := h.db.Model(&models.Project{}).Where("id = ?", lockedProject.ID).Updates(map[string]interface{}{
			"deployment_status":       models.DepStatusQueued,
			"deployment_job_id":       jobID,
			"deployment_message":      msg,
			"deployment_started_at":   &now,
			"deployment_heartbeat_at": &now,
			"deployment_finished_at":  nil,
		}).Error
		if errUpdate != nil {
			slog.Error("Failed to update project deployment status after successful enqueue", "project_id", lockedProject.ID, "error", errUpdate.Error())
		}
	}

	h.databaseService.InvalidateProjectDB(instance.Name)
	h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "completed", "")

	return c.JSON(fiber.Map{
		"success":             true,
		"message":             "Database credentials rotated successfully. Environment synchronization scheduled.",
		"job_id":              jobID,
		"env_sync_generation": envSyncGeneration,
	})
}

// UpdateStatus suspends or resumes database instance
func (h *DatabaseHandler) UpdateStatus(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	var req struct {
		Suspend bool `json:"suspend"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request payload"})
	}

	desiredStatus := models.DBStatusActive
	if req.Suspend {
		desiredStatus = models.DBStatusSuspended
	}
	if desiredStatus == models.DBStatusActive {
		if err := h.requireActiveDatabaseBillingResource(c.UserContext(), instance.ID); err != nil {
			return err
		}
	}
	updatedInstance, err := services.NewDatabaseStatusOperationService(h.db).Request(context.Background(), instance.ID, desiredStatus)
	if err != nil {
		slog.Warn("Database status operation pending retry", "project_id", project.ID, "error", err.Error())
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": false, "message": "Database status change will retry automatically"})
	}
	instance = &updatedInstance

	h.recordAuditLog(c, project.ID, "db_status_change", string(instance.Status), string(instance.Status), "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"status":  instance.Status,
		"message": fmt.Sprintf("Database instance status updated to %s.", instance.Status),
	})
}

func (h *DatabaseHandler) requireActiveDatabaseBillingResource(ctx context.Context, databaseID uint) error {
	if h == nil || h.cfg == nil || !h.cfg.BillingEnabled {
		return nil
	}
	if h.db == nil {
		return apperr.New(503, "BILLING_GATE_UNAVAILABLE", "Billing status is temporarily unavailable")
	}
	var resource models.BillableResource
	err := h.db.WithContext(ctx).
		Where("type = ? AND resource_id = ?", models.BillableTypeDatabase, databaseID).
		First(&resource).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return apperr.New(402, "BILLING_PAYMENT_DUE", "Database resume requires an active billing resource")
	}
	if err != nil {
		return apperr.New(503, "BILLING_GATE_UNAVAILABLE", "Billing status is temporarily unavailable")
	}
	if resource.BillingStatus != models.BillableResourceStatusActive {
		return apperr.New(402, "BILLING_PAYMENT_DUE", "Database resume requires an active billing resource")
	}
	return nil
}

// GetOverview returns metadata summary stats for database dashboard
func (h *DatabaseHandler) GetOverview(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance)
	if err != nil {
		slog.Warn("Failed to connect to project database for overview", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect to database"})
	}

	// Calculate total size and row count
	var totalSizeKB float64
	var totalRows int64
	for _, t := range tables {
		totalRows += t.Rows
		var sz float64
		_, _ = fmt.Sscanf(t.Size, "%f KB", &sz)
		totalSizeKB += sz
	}

	sizeStr := fmt.Sprintf("%.2f KB", totalSizeKB)
	if totalSizeKB > 1024 {
		sizeStr = fmt.Sprintf("%.2f MB", totalSizeKB/1024.0)
	}

	return c.JSON(fiber.Map{
		"engine":       instance.Engine,
		"version":      instance.Version,
		"status":       instance.Status,
		"table_count":  len(tables),
		"row_count":    totalRows,
		"size":         sizeStr,
		"created_at":   instance.CreatedAt,
		"database":     instance.Name,
		"username":     instance.Username,
		"host":         instance.Host,
		"port":         instance.Port,
		"backup_count": 0, // Will be updated by backups router
	})
}

// GetSchema returns visual structure explorer database metadata
func (h *DatabaseHandler) GetSchema(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance)
	if err != nil {
		slog.Warn("Failed to retrieve project database tables", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve tables"})
	}

	columnsMap, fksMap, schemaErr := h.databaseService.GetAllSchemaMetadata(instance)
	if schemaErr != nil {
		slog.Warn("Failed to retrieve project database schema metadata", "project_id", project.ID, "error", schemaErr.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve table metadata"})
	}

	type TableSchema struct {
		Name        string                    `json:"name"`
		Rows        int64                     `json:"rows"`
		Size        string                    `json:"size"`
		Created     string                    `json:"created"`
		Columns     []services.ColumnInfo     `json:"columns"`
		ForeignKeys []services.ForeignKeyInfo `json:"foreign_keys"`
	}

	var schemas []TableSchema
	for _, t := range tables {
		cols := columnsMap[t.Name]
		if cols == nil {
			cols = []services.ColumnInfo{}
		}
		fks := fksMap[t.Name]
		if fks == nil {
			fks = []services.ForeignKeyInfo{}
		}
		schemas = append(schemas, TableSchema{
			Name:        t.Name,
			Rows:        t.Rows,
			Size:        t.Size,
			Created:     t.Created,
			Columns:     cols,
			ForeignKeys: fks,
		})
	}

	return c.JSON(fiber.Map{
		"tables": schemas,
	})
}

// ExecuteDesignerAction handles graphical GUI designer modifications and writes audits
func (h *DatabaseHandler) ExecuteDesignerAction(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	var req services.DesignerActionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request payload"})
	}

	err = h.databaseService.ExecuteDesignerAction(instance, req)
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_designer_"+req.Action, req.TableName, req.NewName, "failed", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database designer operation failed"})
	}

	reqJSON, _ := json.Marshal(req)
	h.recordAuditLog(c, project.ID, "db_designer_"+req.Action, req.TableName, req.NewName, "completed", string(reqJSON))

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Visual designer operation completed successfully.",
	})
}

// ListBackups returns database snapshot backup history catalog
func (h *DatabaseHandler) ListBackups(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	backups, err := h.databaseService.ListBackups(project.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list backups"})
	}

	return c.JSON(fiber.Map{
		"backups": backups,
	})
}

// CreateBackup creates a new manual SQL snapshot of user database
func (h *DatabaseHandler) CreateBackup(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	backup, err := h.databaseService.CreateBackup(project.ID)
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_backup_create", "", "", "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create database backup"})
	}

	h.recordAuditLog(c, project.ID, "db_backup_create", "", backup.Name, "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"backup":  backup,
		"message": "Manual database backup snapshot captured and archived successfully.",
	})
}

// RestoreBackup restores a database state from a catalog snapshot
func (h *DatabaseHandler) RestoreBackup(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	backupID, err := strconv.Atoi(c.Params("backup"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup ID"})
	}

	err = h.databaseService.RestoreBackup(project.ID, uint(backupID))
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_backup_restore", "", strconv.Itoa(backupID), "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Restore failed"})
	}

	h.recordAuditLog(c, project.ID, "db_backup_restore", "", strconv.Itoa(backupID), "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database successfully rolled back and restored to snapshot state.",
	})
}

// DeleteBackup prunes a database snapshot logically and physically
func (h *DatabaseHandler) DeleteBackup(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	backupID, err := strconv.Atoi(c.Params("backup"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup ID"})
	}

	err = h.databaseService.DeleteBackup(project.ID, uint(backupID))
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_backup_delete", "", strconv.Itoa(backupID), "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Prune failed"})
	}

	h.recordAuditLog(c, project.ID, "db_backup_delete", "", strconv.Itoa(backupID), "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database backup catalog record and snapshot file pruned permanently.",
	})
}

// DownloadBackup downloads a physical database snapshot backup file
func (h *DatabaseHandler) DownloadBackup(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	backupID, err := strconv.Atoi(c.Params("backup"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup ID"})
	}

	backup, err := h.databaseService.GetBackupByID(project.ID, uint(backupID))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Backup snapshot not found"})
	}

	if backup.Status != models.BackupStatusCompleted {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Backup snapshot is not completed"})
	}

	// Defense in depth: Check that absolute backup path resides within ProjectsPath
	absProjectsPath, err := filepath.Abs(h.cfg.ProjectsPath)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Internal server config error"})
	}
	absBackupPath, err := filepath.Abs(backup.Path)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Internal server config error"})
	}
	rel, err := filepath.Rel(absProjectsPath, absBackupPath)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Unauthorized file path"})
	}

	h.recordAuditLog(c, project.ID, "db_backup_download", "", strconv.Itoa(backupID), "completed", "")

	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", backup.Name))
	return c.Download(backup.Path)
}

// GetMetrics returns active metrics like connections and size
func (h *DatabaseHandler) GetMetrics(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance)
	if err != nil {
		slog.Warn("Failed to list project database tables for metrics", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve database metrics"})
	}

	var totalSizeKB float64
	for _, t := range tables {
		var sz float64
		_, _ = fmt.Sscanf(t.Size, "%f KB", &sz)
		totalSizeKB += sz
	}

	// Enforce dynamic active connections check in the engine container
	db, err := h.databaseService.ConnectToDatabaseInstance(instance)
	activeConnections := 0
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if instance.Engine == "postgresql" {
			// Exclude connections originating from the Runara backend itself (using secure salted app name)
			appHash := sha256.Sum256([]byte(h.cfg.UIDSalt))
			appName := fmt.Sprintf("paas-backend-%x", appHash[:8])

			_ = db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = $1 AND application_name != $2", instance.Name, appName).Scan(&activeConnections)
		} else {
			// MySQL: Exclude connections originating from the backend container IP
			rows, queryErr := db.QueryContext(ctx, "SELECT HOST FROM information_schema.processlist WHERE db = ?", instance.Name)
			if queryErr == nil {
				defer rows.Close()

				// Thread-safely cache backend IPs to avoid costly interface lookups on every poll
				h.ipsOnce.Do(func() {
					if addrs, errIP := net.InterfaceAddrs(); errIP == nil {
						for _, addr := range addrs {
							if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
								if ipnet.IP.To4() != nil {
									h.localIPs = append(h.localIPs, ipnet.IP.String())
								}
							}
						}
					}
				})

				for rows.Next() {
					var host string
					if errScan := rows.Scan(&host); errScan == nil {
						clientIP := host
						if idx := strings.Index(host, ":"); idx != -1 {
							clientIP = host[:idx]
						}

						isBackend := false
						if clientIP == "localhost" || clientIP == "127.0.0.1" {
							isBackend = true
						} else {
							for _, ip := range h.localIPs {
								if clientIP == ip {
									isBackend = true
									break
								}
							}
						}

						if !isBackend {
							activeConnections++
						}
					}
				}

				// Verify if any connection error occurred during iteration
				if errErr := rows.Err(); errErr != nil {
					slog.Warn("Error occurred during processlist iteration", "db", instance.Name, "error", errErr)
				}
			}
		}
	}

	return c.JSON(fiber.Map{
		"active_connections": activeConnections,
		"size_kb":            totalSizeKB,
	})
}

// TransferDatabase transfers database instance from one project to another project
func (h *DatabaseHandler) TransferDatabase(c *fiber.Ctx) error {
	// 1. Get source project
	sourceProject, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Source project not found"})
	}

	// 2. Parse target project ID
	var req struct {
		TargetProjectID string `json:"target_project_id"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request payload"})
	}

	if req.TargetProjectID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Target project ID is required"})
	}

	// 3. Fetch target project
	targetProject, err := h.projectService.GetProjectByUID(req.TargetProjectID)
	if err != nil || targetProject == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Target project not found"})
	}

	// 4. Validate ownership of target project
	uidVal := c.Locals("user_id")
	roleVal := c.Locals("role")
	if uidVal == nil || roleVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: missing user context"})
	}
	userID := uidVal.(uint)
	role := models.Role(roleVal.(string))

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && targetProject.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "You do not have permission to access the target project"})
	}
	if targetProject.UserID != sourceProject.UserID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Database transfer is limited to projects owned by the same user"})
	}

	// 5. Fetch source database instance
	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(sourceProject.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned for source project"})
	}
	oldProjectID := instance.ProjectID

	var sourceEnvGeneration, targetEnvGeneration uint
	err = services.WithDatabaseCleanupIdentityLock(context.Background(), h.db, instance.Engine, instance.Name, instance.Username, func(lockDB *gorm.DB) error {
		return lockDB.Transaction(func(tx *gorm.DB) error {
			lockedProjects := []models.Project{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []uint{sourceProject.ID, targetProject.ID}).Order("id ASC").Find(&lockedProjects).Error; err != nil {
				return err
			}
			if len(lockedProjects) != 2 {
				return gorm.ErrRecordNotFound
			}
			var lockedSource, lockedTarget *models.Project
			for index := range lockedProjects {
				if lockedProjects[index].ID == sourceProject.ID {
					lockedSource = &lockedProjects[index]
				} else {
					lockedTarget = &lockedProjects[index]
				}
			}
			if lockedSource == nil || lockedTarget == nil || lockedSource.UserID != lockedTarget.UserID {
				return fiber.ErrForbidden
			}
			var lockedInstance models.DatabaseInstance
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", instance.ID, lockedSource.ID).First(&lockedInstance).Error; err != nil {
				return err
			}
			var targetCount int64
			if err := tx.Model(&models.DatabaseInstance{}).Where("project_id = ? AND id <> ? AND status <> ?", lockedTarget.ID, lockedInstance.ID, models.DBStatusDeleted).Count(&targetCount).Error; err != nil {
				return err
			}
			if targetCount > 0 {
				return fiber.ErrConflict
			}
			if err := tx.Model(&lockedInstance).Update("project_id", lockedTarget.ID).Error; err != nil {
				return fmt.Errorf("transfer database instance: %w", err)
			}
			if err := tx.Model(lockedSource).Updates(map[string]any{"database_name": nil, "database_password": "", "database_option": "none"}).Error; err != nil {
				return fmt.Errorf("clear source database credentials: %w", err)
			}
			if err := tx.Model(lockedTarget).Updates(map[string]any{"database_name": lockedInstance.Name, "database_password": lockedInstance.Password, "database_option": "existing"}).Error; err != nil {
				return fmt.Errorf("set target database credentials: %w", err)
			}
			var err error
			sourceEnvGeneration, err = services.RequestProjectEnvSyncTx(tx, lockedSource.ID)
			if err != nil {
				return err
			}
			targetEnvGeneration, err = services.RequestProjectEnvSyncTx(tx, lockedTarget.ID)
			return err
		})
	})
	if err != nil {
		if errors.Is(err, fiber.ErrConflict) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Target project already has an active database instance"})
		}
		if errors.Is(err, fiber.ErrForbidden) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Database transfer is limited to projects owned by the same user"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to transfer database ownership"})
	}

	// 8. Record audit log
	oldProjIDStr := "nil"
	if oldProjectID != nil {
		oldProjIDStr = strconv.Itoa(int(*oldProjectID))
	}
	h.recordAuditLog(c, sourceProject.ID, "db_transfer", oldProjIDStr, strconv.Itoa(int(targetProject.ID)), "completed", "")

	// 9. Durable environment synchronization for both projects.
	sourceJobID, err := h.redisService.EnqueueDeploymentEnvSync(sourceProject.ID, sourceProject.UserID, sourceEnvGeneration)
	if err != nil {
		slog.Warn("Failed to enqueue source project update_env deployment", "project_id", sourceProject.ID, "error", err)
	}

	targetJobID, err := h.redisService.EnqueueDeploymentEnvSync(targetProject.ID, targetProject.UserID, targetEnvGeneration)
	if err != nil {
		slog.Warn("Failed to enqueue target project update_env deployment", "project_id", targetProject.ID, "error", err)
	}

	return c.JSON(fiber.Map{
		"success":       true,
		"message":       "Database ownership transferred successfully and environment updates enqueued.",
		"source_job_id": sourceJobID,
		"target_job_id": targetJobID,
	})
}

// ListTables returns all tables in the database (Legacy/Fallback)
func (h *DatabaseHandler) ListTables(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance)
	if err != nil {
		slog.Warn("Failed to list project tables", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to connect to project database. Please ensure your project is running.",
		})
	}

	return c.JSON(fiber.Map{"tables": tables})
}

// GetTableStructure returns columns for a table (Legacy/Fallback)
func (h *DatabaseHandler) GetTableStructure(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	tableName := c.Params("table")
	if tableName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Table name required"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	columns, err := h.databaseService.GetTableStructure(instance, tableName)
	if err != nil {
		slog.Warn("Failed to get table structure", "project_id", project.ID, "table", tableName, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve table structure"})
	}

	return c.JSON(fiber.Map{"columns": columns})
}

// GetTableData returns rows from a table with pagination (Legacy/Fallback)
func (h *DatabaseHandler) GetTableData(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	tableName := c.Params("table")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	columns, rows, total, err := h.databaseService.GetTableData(instance, tableName, page, limit)
	if err != nil {
		slog.Warn("Failed to get table data", "project_id", project.ID, "table", tableName, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve table data"})
	}

	return c.JSON(fiber.Map{
		"columns": columns,
		"rows":    rows,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// DeleteTableRow deletes a specific row from a table (Legacy/Fallback)
func (h *DatabaseHandler) DeleteTableRow(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	tableName := c.Params("table")
	if tableName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Table name required"})
	}

	var req DeleteTableRowRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.PrimaryKey == "" || req.Value == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "primary_key and value are required"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	deleted, err := h.databaseService.DeleteTableRow(instance, tableName, req.PrimaryKey, req.Value)
	if err != nil {
		slog.Warn("Failed to delete table row", "project_id", project.ID, "table", tableName, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete row"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"deleted": deleted,
	})
}

// UpdateTableRow updates specific fields of a row in a table using a primary key filter
func (h *DatabaseHandler) UpdateTableRow(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	tableName := c.Params("table")
	if tableName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Table name required"})
	}

	var req struct {
		PrimaryKey string                 `json:"primary_key"`
		Value      interface{}            `json:"value"`
		Updates    map[string]interface{} `json:"updates"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request payload"})
	}

	if req.PrimaryKey == "" || req.Value == nil || len(req.Updates) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "primary_key, value, and updates map are required"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	updated, err := h.databaseService.UpdateTableRow(instance, tableName, req.PrimaryKey, req.Value, req.Updates)
	if err != nil {
		slog.Warn("Failed to update table row", "project_id", project.ID, "table", tableName, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update row"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"updated": updated,
	})
}

// ExecuteQuery runs a manual raw query (Legacy/Fallback)
func (h *DatabaseHandler) ExecuteQuery(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	var req ExecuteQueryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	result, err := h.databaseService.ExecuteRawQuery(instance, req.Query)
	if err != nil {
		appErr, ok := err.(*apperr.AppError)
		if ok && appErr != nil {
			return c.Status(appErr.HTTPStatus).JSON(fiber.Map{
				"error": appErr.Message,
				"code":  appErr.Code,
			})
		}
		slog.Error("Database query execution failed", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Query execution failed"})
	}

	return c.JSON(result)
}

// ExportDatabase exports database as SQL (Legacy/Fallback)
func (h *DatabaseHandler) ExportDatabase(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	dump, err := h.databaseService.GenerateProjectDump(instance)
	if err != nil {
		slog.Warn("Failed to export database", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to export database"})
	}

	c.Set("Content-Type", "application/sql")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.sql", project.GetDatabaseName()))
	return c.SendString(dump)
}

// ImportDatabase imports SQL file (Legacy/Fallback)
func (h *DatabaseHandler) ImportDatabase(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	var req ImportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.SQL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SQL content is required"})
	}
	if len(req.SQL) > maxDatabaseImportBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(fiber.Map{"error": "SQL import payload is too large"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	statements := services.SplitSQLStatements(req.SQL)
	if len(statements) > maxDatabaseImportStatements {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SQL import contains too many statements"})
	}
	successCount := 0
	var errors []string

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		res, err := h.databaseService.ExecuteRawQuery(instance, stmt)
		if err != nil {
			errors = append(errors, "Statement execution failed")
		} else {
			if res.RowsAffected > 0 || res.Columns != nil {
				successCount++
			}
		}
	}

	return c.JSON(fiber.Map{
		"success":    len(errors) == 0,
		"statements": successCount,
		"errors":     errors,
	})
}

// ResetDatabase drops all tables (Legacy/Fallback)
func (h *DatabaseHandler) ResetDatabase(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	dropped, err := h.databaseService.ResetProjectDatabase(instance)
	if err != nil {
		slog.Warn("Failed to reset database", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reset database"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"dropped": dropped,
	})
}

// AdminListAll returns all user databases (Admin only)
func (h *DatabaseHandler) AdminListAll(c *fiber.Ctx) error {
	databases, err := h.databaseService.AdminListAllDatabases()
	if err != nil {
		slog.Error("Failed to list all databases", "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve database list"})
	}

	return c.JSON(fiber.Map{"databases": databases})
}

type DeleteTableRowRequest struct {
	PrimaryKey string      `json:"primary_key"`
	Value      interface{} `json:"value"`
}

type ExecuteQueryRequest struct {
	Query string `json:"query"`
}

type ImportRequest struct {
	SQL string `json:"sql"`
}

// ListUserDatabases lists all databases owned by the authenticated user
func (h *DatabaseHandler) ListUserDatabases(c *fiber.Ctx) error {
	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	var databases []models.DatabaseInstance
	if err := h.db.Preload("Project").Where("user_id = ? AND status != ?", userID, models.DBStatusDeleted).Find(&databases).Error; err != nil {
		slog.Error("Failed to list user databases", "userID", userID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to list databases"})
	}

	return c.JSON(fiber.Map{
		"databases": databases,
	})
}

// AttachDatabase attaches a database to a project
func (h *DatabaseHandler) AttachDatabase(c *fiber.Ctx) error {
	dbUID := c.Params("uid")
	if dbUID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database UID is required"})
	}

	var req struct {
		ProjectUID string `json:"project_uid"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Fetch DatabaseInstance and validate ownership (pre-check for fast 404/403)
	var dbInst models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
	}
	if dbInst.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}

	// Fetch project by UID and validate ownership (pre-check for fast 404/403)
	project, err := h.projectService.GetProjectByUID(req.ProjectUID)
	if err != nil || project == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	if project.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this project"})
	}

	var envSyncGeneration uint
	// Locked transaction: re-fetch and recheck to prevent concurrent attach race
	errTx := h.db.Transaction(func(tx *gorm.DB) error {
		// Lock database row and recheck availability
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", dbInst.ID).First(&dbInst).Error; err != nil {
			return fmt.Errorf("database not found")
		}
		if dbInst.Status != models.DBStatusActive {
			return fmt.Errorf("database is not active")
		}
		if dbInst.ProjectID != nil {
			return fmt.Errorf("database is already attached to a project")
		}

		// Lock project row and recheck no DB already attached
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(project, project.ID).Error; err != nil {
			return fmt.Errorf("project not found")
		}
		var count int64
		tx.Model(&models.DatabaseInstance{}).Where("project_id = ?", project.ID).Count(&count)
		if count > 0 {
			return fmt.Errorf("project already has a database attached")
		}

		projID := project.ID
		dbInst.ProjectID = &projID
		if err := tx.Save(&dbInst).Error; err != nil {
			return err
		}

		project.DatabaseName = &dbInst.Name
		project.DatabasePassword = dbInst.Password
		project.DatabaseOption = "existing"
		if err := tx.Save(project).Error; err != nil {
			return err
		}
		generation, err := services.RequestProjectEnvSyncTx(tx, project.ID)
		if err != nil {
			return err
		}
		envSyncGeneration = generation

		return nil
	})

	if errTx != nil {
		h.recordAuditLog(c, project.ID, "db_attach", "active", "active", "failed", errTx.Error())
		// Return 409 for race-detectable conflicts, 500 for other failures
		errMsg := errTx.Error()
		switch errMsg {
		case "database is already attached to a project", "project already has a database attached", "database is not active":
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": errMsg})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to attach database: " + errMsg})
	}

	if _, err := h.redisService.EnqueueDeploymentEnvSync(project.ID, project.UserID, envSyncGeneration); err != nil {
		slog.Warn("Queue durable database attach environment sync failed", "project_id", project.ID, "generation", envSyncGeneration, "error", err)
	}
	// Propagate env updates (after commit). Report enqueue failure in audit.
	if err := h.secretStoreService.PropagateDatabaseEnv(project); err != nil {
		h.recordAuditLog(c, project.ID, "db_attach", "active", "active", "env_sync_pending", "env propagation enqueue failed: "+err.Error())
		return c.JSON(fiber.Map{"success": true, "message": "Database attached successfully", "env_sync": "pending"})
	}

	h.recordAuditLog(c, project.ID, "db_attach", "active", "active", "completed", "")
	return c.JSON(fiber.Map{"success": true, "message": "Database attached successfully"})
}

// DetachDatabase detaches a database from its project
func (h *DatabaseHandler) DetachDatabase(c *fiber.Ctx) error {
	dbUID := c.Params("uid")
	if dbUID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database UID is required"})
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Fetch DatabaseInstance and validate ownership (pre-check for fast 404/403)
	var dbInst models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
	}
	if dbInst.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}

	if dbInst.ProjectID == nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database is not attached to any project"})
	}

	projectID := *dbInst.ProjectID

	// Fetch project (pre-check for fast 404/403)
	var project models.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if project.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own the project associated with this database"})
	}

	// Locked transaction: re-fetch and recheck to prevent concurrent detach race
	var envSyncGeneration uint
	errTx := services.WithDatabaseCleanupIdentityLock(context.Background(), h.db, dbInst.Engine, dbInst.Name, dbInst.Username, func(lockDB *gorm.DB) error {
		return lockDB.Transaction(func(tx *gorm.DB) error {
			// Lock database row and recheck it's still attached
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&dbInst, dbInst.ID).Error; err != nil {
				return fmt.Errorf("database not found")
			}
			if dbInst.ProjectID == nil {
				return fmt.Errorf("database is not attached to any project")
			}

			// Lock project row
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&project, project.ID).Error; err != nil {
				return fmt.Errorf("project not found")
			}

			dbInst.ProjectID = nil
			if err := tx.Save(&dbInst).Error; err != nil {
				return err
			}

			project.DatabaseName = nil
			project.DatabasePassword = ""
			project.DatabaseOption = "none"
			if err := tx.Save(&project).Error; err != nil {
				return err
			}
			generation, err := services.RequestProjectEnvSyncTx(tx, project.ID)
			if err != nil {
				return err
			}
			envSyncGeneration = generation

			return nil
		})
	})

	if errTx != nil {
		h.recordAuditLog(c, projectID, "db_detach", "active", "active", "failed", errTx.Error())
		errMsg := errTx.Error()
		if errMsg == "database is not attached to any project" {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": errMsg})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to detach database: " + errMsg})
	}

	if _, err := h.redisService.EnqueueDeploymentEnvSync(project.ID, project.UserID, envSyncGeneration); err != nil {
		slog.Warn("Queue durable database detach environment sync failed", "project_id", project.ID, "generation", envSyncGeneration, "error", err)
	}
	// Propagate env updates (removes DB_* vars since association is now nil, after commit)
	if err := h.secretStoreService.PropagateDatabaseEnv(&project); err != nil {
		h.recordAuditLog(c, projectID, "db_detach", "active", "active", "env_sync_pending", "env propagation enqueue failed: "+err.Error())
		return c.JSON(fiber.Map{"success": true, "message": "Database detached successfully", "env_sync": "pending"})
	}

	h.recordAuditLog(c, projectID, "db_detach", "active", "active", "completed", "")
	return c.JSON(fiber.Map{"success": true, "message": "Database detached successfully"})
}

// ResetDatabaseInstance wipes all data in the database
func (h *DatabaseHandler) ResetDatabaseInstance(c *fiber.Ctx) error {
	dbUID := c.Params("uid")
	if dbUID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database UID is required"})
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	// Fetch DatabaseInstance and validate ownership
	var dbInst models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
	}
	if dbInst.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}
	if dbInst.Status != models.DBStatusActive {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Database is not active"})
	}
	if err := h.requireDatabaseRuntimeAction(c, &dbInst); err != nil {
		return err
	}

	// Connect and reset
	conn, err := h.databaseService.ConnectToDatabaseInstance(&dbInst)
	if err != nil {
		slog.Error("Failed to connect to database for reset", "db", dbInst.Name, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect to database"})
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if dbInst.Engine == "postgresql" {
		if !h.databaseService.IsValidIdentifier(dbInst.Username) {
			slog.Error("Invalid database username identifier for reset", "db", dbInst.Name, "username", dbInst.Username)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid database username identifier"})
		}
		escapedUsername := h.databaseService.EscapeIdentifier("postgresql", dbInst.Username)

		if _, err := conn.ExecContext(ctx, "DROP SCHEMA public CASCADE;"); err != nil {
			slog.Error("Failed to drop schema for postgresql reset", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reset database: " + err.Error()})
		}
		if _, err := conn.ExecContext(ctx, "CREATE SCHEMA public;"); err != nil {
			slog.Error("Failed to recreate schema for postgresql reset", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reset database: " + err.Error()})
		}
		// Also restore public permissions just in case
		_, _ = conn.ExecContext(ctx, "GRANT ALL ON SCHEMA public TO public;")
		_, _ = conn.ExecContext(ctx, fmt.Sprintf("GRANT ALL ON SCHEMA public TO %s;", escapedUsername))
	} else {
		// MySQL reset
		_, err := h.databaseService.ResetProjectDatabase(&dbInst)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reset database: " + err.Error()})
		}
	}

	h.recordAuditLog(c, 0, "db_reset", "active", "active", "completed", "")
	return c.JSON(fiber.Map{"success": true, "message": "Database wiped clean successfully"})
}

// ReinstallDatabaseInstance drops and recreates the database schema, rotating credentials
func (h *DatabaseHandler) ReinstallDatabaseInstance(c *fiber.Ctx) error {
	dbUID := c.Params("uid")
	if dbUID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database UID is required"})
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	var instance models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load database"})
	}
	if instance.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}
	if err := h.requireDatabaseRuntimeAction(c, &instance); err != nil {
		return err
	}

	project, instance, err := services.NewDatabaseReinstallRecoveryService(h.db, h.secretStoreService).
		StartOrResumeDatabaseReinstall(context.Background(), dbUID, userID)

	if err != nil {
		nextStatus := "active"
		var task models.DatabaseReinstallRecoveryTask
		if h.db.Where("database_instance_uid = ?", dbUID).First(&task).Error == nil {
			nextStatus = "suspended"
		}
		h.recordAuditLog(c, 0, "db_reinstall", "active", nextStatus, "failed", err.Error())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
		}
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reinstall database"})
	}

	h.databaseService.InvalidateProjectDB(instance.Name)
	h.recordAuditLog(c, 0, "db_reinstall", "active", "active", "completed", "")
	return c.JSON(fiber.Map{
		"success":          true,
		"message":          "Database reinstalled and password rotated successfully",
		"redeployRequired": project.ID != 0,
	})
}

func (h *DatabaseHandler) requireDatabaseRuntimeAction(c *fiber.Ctx, instance *models.DatabaseInstance) error {
	if h == nil || h.cfg == nil || !h.cfg.BillingEnabled || instance == nil {
		return nil
	}
	err := billinggate.NewProjectRuntimeGate(h.db, true, h.cfg.BillingDeployBlockDays).CheckResource(c.UserContext(), models.BillableTypeDatabase, instance.ID, time.Now().UTC())
	switch {
	case err == nil:
		return nil
	case errors.Is(err, billinggate.ErrProjectActionBlocked), errors.Is(err, billinggate.ErrPaymentDueEvidenceUnavailable):
		return apperr.New(402, "BILLING_PAYMENT_DUE", "Database actions are blocked until the overdue invoice is paid")
	default:
		return apperr.New(503, "BILLING_GATE_UNAVAILABLE", "Billing status is temporarily unavailable")
	}
}

// CreateDatabase request body definition
type CreateDatabaseRequest struct {
	Engine         string `json:"engine"`
	Name           string `json:"name"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	BillableSpecID uint   `json:"billable_spec_id"`
}

// CreateDatabase handles creation of a standalone database instance
func (h *DatabaseHandler) CreateDatabase(c *fiber.Ctx) (handlerErr error) {
	var req CreateDatabaseRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	if h.cfg.BillingEnabled && req.BillableSpecID == 0 {
		return apperr.NewBadRequest("A database billing specification is required")
	}

	// Validate engine
	var err error
	req.Engine, err = utils.ValidateDatabaseEngine(req.Engine)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Trim space if desired, and reject casing drift explicitly before mutation
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)

	// Validate name
	if err := utils.ValidateDatabaseName(req.Name); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate username
	if err := utils.ValidateDatabaseUsername(req.Username); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate password
	if err := utils.ValidateDatabasePassword(req.Password); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	// Check global uniqueness for Database Name and Username across all non-deleted database instances
	var duplicateCount int64
	h.db.Model(&models.DatabaseInstance{}).Where("name = ? AND status != ?", req.Name, models.DBStatusDeleted).Count(&duplicateCount)
	if duplicateCount > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": fmt.Sprintf("A database with name '%s' already exists in the system", req.Name)})
	}

	var duplicateUserCount int64
	h.db.Model(&models.DatabaseInstance{}).Where("username = ? AND status != ?", req.Username, models.DBStatusDeleted).Count(&duplicateUserCount)
	if duplicateUserCount > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": fmt.Sprintf("A database user with username '%s' already exists in the system", req.Username)})
	}

	// Determine host & port based on engine
	var host string
	var port int
	if req.Engine == "postgresql" {
		host = infrastructure.PostgreSQLContainerName()
		port = infrastructure.PostgreSQLPort()
	} else {
		host = infrastructure.MySQLContainerName()
		port = infrastructure.MySQLPort()
	}

	dbInst := &models.DatabaseInstance{
		UserID:    userID,
		Engine:    req.Engine,
		Name:      req.Name,
		Username:  req.Username, // Use actual requested username instead of hardcoded req.Name
		Password:  req.Password,
		Host:      host,
		Port:      port,
		Status:    models.DBStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	var outcome standaloneDatabaseProvisionOutcome
	defer func() {
		if recovered := recover(); recovered != nil {
			if outcome.SafeToCompensate && !outcome.DatabaseInstancePersisted && outcome.Ownership.HasResources() {
				compensateStandaloneDatabase(h.db, req.Engine, req.Name, req.Username, outcome.Ownership)
			}
			slog.Error("Database provisioning panicked", "engine", req.Engine, "database_name", req.Name, "error", recovered)
			handlerErr = apperr.New(500, "DATABASE_PROVISIONING_FAILED", "Failed to provision managed database")
		}
	}()
	provisionErr := services.WithDatabaseCleanupIdentityLock(context.Background(), h.db, req.Engine, req.Name, req.Username, func(controlDB *gorm.DB) error {
		if err := ensureNoDatabaseCleanupTask(controlDB, req.Engine, req.Name, req.Username); err != nil {
			return err
		}
		if err := createStandaloneProvisioningIntent(controlDB, req.Engine, req.Name, req.Username); err != nil {
			return err
		}
		var err error
		outcome, err = h.persistAndProvisionStandaloneDatabase(controlDB, h.db, dbInst, req, userID)
		if err != nil {
			return releaseStandaloneProvisioningIntent(controlDB, req.Engine, req.Name, req.Username, err)
		}
		if err := completeStandaloneProvisioningIntent(controlDB, req.Engine, req.Name, req.Username); err != nil {
			slog.Error("Database provisioning intent cleanup deferred after committed provisioning", "engine", req.Engine, "database_name", req.Name, "error", err)
		}
		return nil
	})
	if provisionErr != nil {
		if outcome.SafeToCompensate && !outcome.DatabaseInstancePersisted && outcome.Ownership.HasResources() {
			compensateStandaloneDatabase(h.db, req.Engine, req.Name, req.Username, outcome.Ownership)
		}
		switch {
		case errors.Is(provisionErr, billing.ErrInsufficientCredits), errors.Is(provisionErr, billing.ErrInvalidInvoiceInput), errors.Is(provisionErr, billing.ErrResourceAlreadyBilled):
			return mapDatabaseBillingError(provisionErr)
		}
		var appErr *apperr.AppError
		if errors.As(provisionErr, &appErr) {
			return appErr
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to provision managed database"})
	}

	h.recordAuditLog(c, 0, "db_create", "", "active", "completed", "")

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":  true,
		"database": dbInst,
	})
}

type standaloneDatabaseProvisionOutcome struct {
	Ownership                 infrastructure.ProvisioningOwnership
	DatabaseInstancePersisted bool
	SafeToCompensate          bool
}

func (h *DatabaseHandler) persistAndProvisionStandaloneDatabase(db, recoveryDB *gorm.DB, dbInst *models.DatabaseInstance, req CreateDatabaseRequest, userID uint) (standaloneDatabaseProvisionOutcome, error) {
	var outcome standaloneDatabaseProvisionOutcome
	if h.cfg.BillingEnabled {
		tx := db.Begin()
		if tx.Error != nil {
			return outcome, apperr.New(500, "BILLING_TRANSACTION_FAILED", "Failed to prepare database billing")
		}
		quota, err := billing.LoadActiveDatabaseQuotaTx(tx, req.BillableSpecID)
		if err != nil {
			tx.Rollback()
			return outcome, err
		}
		dbInst.ConnectionLimit = quota.ConnectionLimit
		if err := tx.Create(dbInst).Error; err != nil {
			tx.Rollback()
			return outcome, fmt.Errorf("save database record: %w", err)
		}
		invoiceService := billing.NewInvoiceService(db, billing.NewWalletService(db))
		if err := invoiceService.ChargeInitialResourceTx(tx, userID, models.BillableTypeDatabase, dbInst.ID, req.BillableSpecID, time.Now().UTC()); err != nil {
			tx.Rollback()
			return outcome, err
		}
		// ponytail: Phase 5 keeps this transaction open to avoid a charged orphan; use a durable provisioning saga when engine creation becomes asynchronous.
		outcome.Ownership, err = provisionStandaloneDatabaseWithCheckpoint(req.Engine, req.Name, req.Username, req.Password, dbInst.ConnectionLimit, func(ownership infrastructure.ProvisioningOwnership) error {
			return checkpointStandaloneProvisioningOwnership(recoveryDB, req.Engine, req.Name, req.Username, ownership)
		})
		if err != nil {
			tx.Rollback()
			outcome.SafeToCompensate = true
			return outcome, err
		}
		if err := tx.Commit().Error; err != nil {
			persisted, recoveryErr := databaseInstancePersisted(recoveryDB, dbInst)
			if recoveryErr != nil {
				return outcome, fmt.Errorf("verify ambiguous database provisioning commit: %w", recoveryErr)
			}
			if persisted {
				outcome.DatabaseInstancePersisted = true
				return outcome, nil
			}
			outcome.SafeToCompensate = true
			return outcome, fmt.Errorf("finalize database provisioning: %w", err)
		}
		outcome.DatabaseInstancePersisted = true
		return outcome, nil
	}

	var err error
	outcome.Ownership, err = provisionStandaloneDatabaseWithCheckpoint(req.Engine, req.Name, req.Username, req.Password, dbInst.ConnectionLimit, func(ownership infrastructure.ProvisioningOwnership) error {
		return checkpointStandaloneProvisioningOwnership(recoveryDB, req.Engine, req.Name, req.Username, ownership)
	})
	if err != nil {
		outcome.SafeToCompensate = true
		return outcome, err
	}
	if err := db.Create(dbInst).Error; err != nil {
		persisted, recoveryErr := databaseInstancePersisted(recoveryDB, dbInst)
		if recoveryErr != nil {
			return outcome, fmt.Errorf("verify ambiguous database record insert: %w", recoveryErr)
		}
		if persisted {
			outcome.DatabaseInstancePersisted = true
			return outcome, nil
		}
		outcome.SafeToCompensate = true
		return outcome, fmt.Errorf("save database record: %w", err)
	}
	outcome.DatabaseInstancePersisted = true
	return outcome, nil
}

func databaseInstancePersisted(db *gorm.DB, dbInst *models.DatabaseInstance) (bool, error) {
	if db == nil || dbInst == nil || dbInst.ID == 0 || dbInst.UID == "" {
		return false, errors.New("database instance recovery identity is invalid")
	}
	var count int64
	err := db.Connection(func(connection *gorm.DB) error {
		return connection.Model(&models.DatabaseInstance{}).Where("id = ? AND uid = ?", dbInst.ID, dbInst.UID).Count(&count).Error
	})
	if err != nil {
		return false, err
	}
	return count == 1, nil
}

func ensureNoDatabaseCleanupTask(db *gorm.DB, engine, name, username string) error {
	var count int64
	if err := db.Model(&models.DatabaseCleanupTask{}).Where("engine = ? AND name = ? AND username = ?", engine, name, username).Count(&count).Error; err != nil {
		return fmt.Errorf("check pending database cleanup: %w", err)
	}
	if count > 0 {
		return apperr.New(409, "DATABASE_CLEANUP_PENDING", "Database provisioning is temporarily unavailable while prior cleanup completes")
	}
	return nil
}

func compensateStandaloneDatabase(db *gorm.DB, engine, name, username string, ownership infrastructure.ProvisioningOwnership) {
	compensateStandaloneDatabaseWithCleanup(db, engine, name, username, ownership, services.DeprovisionStandaloneDatabase)
}

func compensateStandaloneDatabaseWithCleanup(db *gorm.DB, engine, name, username string, ownership infrastructure.ProvisioningOwnership, cleanup func(string, string, string, infrastructure.ProvisioningOwnership) (infrastructure.ProvisioningOwnership, error)) {
	if !ownership.HasResources() {
		return
	}
	cleanupErr := services.WithDatabaseCleanupIdentityLock(context.Background(), db, engine, name, username, func(lockDB *gorm.DB) error {
		remaining, err := cleanup(engine, name, username, ownership)
		if err != nil {
			if recordErr := recordDatabaseCleanupTask(lockDB, engine, name, username, remaining, err); recordErr != nil {
				return errors.Join(err, fmt.Errorf("record database cleanup task: %w", recordErr))
			}
			slog.Error("Managed database cleanup queued", "engine", engine, "database_name", name, "cleanup_error", err)
		}
		return nil
	})
	if cleanupErr != nil {
		slog.Error("Failed to compensate managed database provisioning", "engine", engine, "database_name", name, "error", cleanupErr)
	}
}

func recordDatabaseCleanupTask(db *gorm.DB, engine, name, username string, ownership infrastructure.ProvisioningOwnership, cleanupErr error) error {
	task := models.DatabaseCleanupTask{Engine: engine, Name: name, Username: username, Reason: models.DatabaseCleanupReasonProvisioning, DatabaseOwned: ownership.DatabaseCreated, UserOwned: ownership.UserCreated}
	return recordDatabaseCleanupTaskWithContext(db, task, cleanupErr)
}

func recordDatabaseDeletionCleanupTask(db *gorm.DB, dbInst models.DatabaseInstance, ownership infrastructure.ProvisioningOwnership, cleanupErr error) error {
	databaseInstanceID := dbInst.ID
	task := models.DatabaseCleanupTask{Engine: dbInst.Engine, Name: dbInst.Name, Username: dbInst.Username, Reason: models.DatabaseCleanupReasonRequestedDeletion, DatabaseInstanceID: &databaseInstanceID, DatabaseInstanceUID: dbInst.UID, DatabaseOwned: ownership.DatabaseCreated, UserOwned: ownership.UserCreated}
	return recordDatabaseCleanupTaskWithContext(db, task, cleanupErr)
}

func recordDatabaseCleanupTaskWithContext(db *gorm.DB, task models.DatabaseCleanupTask, cleanupErr error) error {
	if db == nil || cleanupErr == nil || (task.Reason != models.DatabaseCleanupReasonRequestedDeletion && !task.DatabaseOwned && !task.UserOwned) {
		return errors.New("database cleanup task is invalid")
	}
	task.LastError = cleanupErr.Error()
	task.RetryCount = 1
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "engine"}, {Name: "name"}, {Name: "username"}},
		DoUpdates: clause.Assignments(map[string]any{
			"reason":                task.Reason,
			"database_instance_id":  task.DatabaseInstanceID,
			"database_instance_uid": task.DatabaseInstanceUID,
			"database_owned":        gorm.Expr("database_owned OR ?", task.DatabaseOwned),
			"user_owned":            gorm.Expr("user_owned OR ?", task.UserOwned),
			"last_error":            cleanupErr.Error(),
			"retry_count":           gorm.Expr("retry_count + ?", 1),
		}),
	}).Create(&task).Error
}

func createStandaloneProvisioningIntent(db *gorm.DB, engine, name, username string) error {
	leaseExpiresAt := time.Now().UTC().Add(10 * time.Minute)
	task := models.DatabaseCleanupTask{
		Engine:         engine,
		Name:           name,
		Username:       username,
		Reason:         models.DatabaseCleanupReasonProvisioning,
		DatabaseOwned:  false,
		UserOwned:      false,
		LeaseToken:     "provisioning-intent",
		LeaseExpiresAt: &leaseExpiresAt,
		LastError:      "durable provisioning intent",
	}
	if err := db.Create(&task).Error; err != nil {
		return fmt.Errorf("persist database provisioning intent: %w", err)
	}
	return nil
}

func releaseStandaloneProvisioningIntent(db *gorm.DB, engine, name, username string, provisionErr error) error {
	if provisionErr == nil {
		return errors.New("database provisioning error is required")
	}
	if err := db.Model(&models.DatabaseCleanupTask{}).
		Where("engine = ? AND name = ? AND username = ? AND reason = ?", engine, name, username, models.DatabaseCleanupReasonProvisioning).
		Updates(map[string]any{"lease_token": "", "last_error": provisionErr.Error(), "retry_count": gorm.Expr("retry_count + 1")}).Error; err != nil {
		return errors.Join(provisionErr, fmt.Errorf("release database provisioning intent: %w", err))
	}
	return provisionErr
}

func completeStandaloneProvisioningIntent(db *gorm.DB, engine, name, username string) error {
	if err := db.Where("engine = ? AND name = ? AND username = ? AND reason = ?", engine, name, username, models.DatabaseCleanupReasonProvisioning).
		Delete(&models.DatabaseCleanupTask{}).Error; err != nil {
		return fmt.Errorf("complete database provisioning intent: %w", err)
	}
	return nil
}

func checkpointStandaloneProvisioningOwnership(db *gorm.DB, engine, name, username string, ownership infrastructure.ProvisioningOwnership) error {
	if !ownership.HasResources() {
		return nil
	}
	if db == nil {
		return errors.New("database provisioning ownership checkpoint is unavailable")
	}
	if err := db.Model(&models.DatabaseCleanupTask{}).
		Where("engine = ? AND name = ? AND username = ? AND reason = ?", engine, name, username, models.DatabaseCleanupReasonProvisioning).
		Updates(map[string]any{
			"database_owned": gorm.Expr("database_owned OR ?", ownership.DatabaseCreated),
			"user_owned":     gorm.Expr("user_owned OR ?", ownership.UserCreated),
			"last_error":     "provisioning ownership checkpointed",
		}).Error; err != nil {
		return fmt.Errorf("checkpoint database provisioning ownership: %w", err)
	}
	return nil
}

func provisionStandaloneDatabaseWithCheckpoint(engine, name, username, password string, connectionLimit int, checkpoint func(infrastructure.ProvisioningOwnership) error) (infrastructure.ProvisioningOwnership, error) {
	if connectionLimit <= 0 {
		connectionLimit = infrastructure.DefaultManagedDatabaseConnectionLimit
	}
	if engine == "postgresql" {
		return infrastructure.NewPostgreSQLService().ProvisionDatabaseCustomWithConnectionLimitCheckpoint(name, username, password, connectionLimit, checkpoint)
	}
	return infrastructure.NewMySQLService().ProvisionDatabaseCustomWithConnectionLimitCheckpoint(name, username, password, connectionLimit, checkpoint)
}

func mapDatabaseBillingError(err error) error {
	switch {
	case errors.Is(err, billing.ErrInsufficientCredits):
		return apperr.New(402, "INSUFFICIENT_CREDITS", "Insufficient Runa's Tokens to provision this resource")
	case errors.Is(err, billing.ErrInvalidInvoiceInput):
		return apperr.NewBadRequest("Invalid billing specification")
	case errors.Is(err, billing.ErrResourceAlreadyBilled):
		return apperr.New(409, "BILLABLE_RESOURCE_CONFLICT", "Resource billing is already initialized")
	default:
		return err
	}
}

// DeleteDatabase handles deletion of a database instance (only if unattached)
func (h *DatabaseHandler) DeleteDatabase(c *fiber.Ctx) error {
	dbUID := c.Params("uid")
	if dbUID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database UID is required"})
	}

	uidVal := c.Locals("user_id")
	if uidVal == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	userID, ok := uidVal.(uint)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	dbInst, err := h.suspendStandaloneDatabaseForDeletion(dbUID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
		}
		var appErr *apperr.AppError
		if errors.As(err, &appErr) {
			return appErr
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to prepare database deletion"})
	}

	deprovisionErr := services.WithDatabaseCleanupIdentityLock(context.Background(), h.db, dbInst.Engine, dbInst.Name, dbInst.Username, func(lockDB *gorm.DB) error {
		remaining, err := services.DeprovisionStandaloneDatabase(dbInst.Engine, dbInst.Name, dbInst.Username, infrastructure.ProvisioningOwnership{DatabaseCreated: true, UserCreated: true})
		if err == nil {
			return nil
		}
		if recordErr := recordDatabaseDeletionCleanupTask(lockDB, dbInst, remaining, err); recordErr != nil {
			return errors.Join(err, fmt.Errorf("record database deletion cleanup: %w", recordErr))
		}
		return err
	})
	if deprovisionErr != nil {
		slog.Error("Physical database deletion queued for retry", "database_id", dbInst.ID, "error", deprovisionErr)
		h.recordAuditLog(c, 0, "db_delete", "active", "suspended", "pending_cleanup", deprovisionErr.Error())
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": false, "message": "Database billing has been suspended; physical deletion will retry automatically"})
	}

	if err := services.FinalizeDatabaseDeletion(context.Background(), h.db, dbInst.ID, dbInst.UID, h.cfg.ProjectsPath); err != nil {
		recordErr := services.WithDatabaseCleanupIdentityLock(context.Background(), h.db, dbInst.Engine, dbInst.Name, dbInst.Username, func(lockDB *gorm.DB) error {
			return recordDatabaseDeletionCleanupTask(lockDB, dbInst, infrastructure.ProvisioningOwnership{}, err)
		})
		if recordErr != nil {
			slog.Error("Failed to queue database deletion finalization", "database_id", dbInst.ID, "finalize_error", err, "record_error", recordErr)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Physical database deletion completed, but finalization could not be queued"})
		}
		slog.Error("Database deletion finalization queued for retry", "database_id", dbInst.ID, "error", err)
		h.recordAuditLog(c, 0, "db_delete", "suspended", "suspended", "pending_finalization", err.Error())
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": false, "message": "Physical deletion completed; logical cleanup will retry automatically"})
	}
	if err := h.db.Where("engine = ? AND name = ? AND username = ? AND reason = ?", dbInst.Engine, dbInst.Name, dbInst.Username, models.DatabaseCleanupReasonRequestedDeletion).Delete(&models.DatabaseCleanupTask{}).Error; err != nil {
		slog.Error("Database deletion finalized but retry task remains", "database_id", dbInst.ID, "error", err)
		return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"success": false, "message": "Database deletion completed; retry finalization will confirm completion automatically"})
	}

	h.recordAuditLog(c, 0, "db_delete", "active", "deleted", "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database deleted successfully",
	})
}

func (h *DatabaseHandler) suspendStandaloneDatabaseForDeletion(dbUID string, userID uint) (models.DatabaseInstance, error) {
	var dbInst models.DatabaseInstance
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
			return err
		}
		if dbInst.UserID != userID {
			return apperr.New(403, "DATABASE_FORBIDDEN", "Forbidden: You do not own this database")
		}
		if dbInst.Status == models.DBStatusDeleted {
			return gorm.ErrRecordNotFound
		}
		if dbInst.ProjectID != nil {
			return apperr.New(409, "DATABASE_ATTACHED", "Database is currently attached to a project. Detach it first.")
		}

		var resource models.BillableResource
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("type = ? AND resource_id = ?", models.BillableTypeDatabase, dbInst.ID).First(&resource).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("lock database billable resource: %w", err)
		}
		if err == nil && resource.BillingStatus != models.BillableResourceStatusDeleted {
			if err := tx.Model(&resource).Update("billing_status", models.BillableResourceStatusDeleted).Error; err != nil {
				return fmt.Errorf("terminate database billing: %w", err)
			}
		}
		if dbInst.Status != models.DBStatusSuspended {
			if err := tx.Model(&dbInst).Update("status", models.DBStatusSuspended).Error; err != nil {
				return fmt.Errorf("suspend database instance: %w", err)
			}
			dbInst.Status = models.DBStatusSuspended
		}
		databaseInstanceID := dbInst.ID
		task := models.DatabaseCleanupTask{Engine: dbInst.Engine, Name: dbInst.Name, Username: dbInst.Username, Reason: models.DatabaseCleanupReasonRequestedDeletion, DatabaseInstanceID: &databaseInstanceID, DatabaseInstanceUID: dbInst.UID, DatabaseOwned: true, UserOwned: true, LastError: "requested deletion pending"}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "engine"}, {Name: "name"}, {Name: "username"}}, DoUpdates: clause.Assignments(map[string]any{"reason": models.DatabaseCleanupReasonRequestedDeletion, "database_instance_id": databaseInstanceID, "database_instance_uid": dbInst.UID, "database_owned": true, "user_owned": true, "lease_token": "", "lease_expires_at": nil, "last_error": "requested deletion pending"})}).Create(&task).Error; err != nil {
			return fmt.Errorf("upsert requested database deletion task: %w", err)
		}
		return nil
	})
	return dbInst, err
}
