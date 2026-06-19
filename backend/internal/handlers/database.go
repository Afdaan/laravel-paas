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
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	maxDatabaseImportBytes              = 5 * 1024 * 1024
	maxDatabaseImportStatements         = 200
	databasePasswordInvalidCharsMessage = "Database password must not contain spaces or connection-string-breaking characters like \", ', `, \\, ;, @, #, /, or ?"
)

var reservedDatabaseWords = map[string]struct{}{
	"all": {}, "alter": {}, "and": {}, "any": {}, "as": {}, "asc": {}, "authorization": {},
	"backup": {}, "begin": {}, "between": {}, "break": {}, "browse": {}, "bulk": {}, "by": {},
	"cascade": {}, "case": {}, "check": {}, "checkpoint": {}, "close": {}, "clustered": {},
	"coalesce": {}, "collate": {}, "column": {}, "commit": {}, "compute": {}, "constraint": {},
	"contains": {}, "containstable": {}, "continue": {}, "convert": {}, "create": {}, "cross": {},
	"current": {}, "current_date": {}, "current_time": {}, "current_timestamp": {}, "current_user": {},
	"cursor": {}, "database": {}, "dbcc": {}, "deallocate": {}, "declare": {}, "default": {},
	"delete": {}, "deny": {}, "desc": {}, "disk": {}, "distinct": {}, "distributed": {},
	"double": {}, "drop": {}, "dump": {}, "else": {}, "end": {}, "errlvl": {}, "escape": {},
	"except": {}, "exec": {}, "execute": {}, "exists": {}, "exit": {}, "external": {},
	"fetch": {}, "file": {}, "fillfactor": {}, "for": {}, "foreign": {}, "freetext": {},
	"freetexttable": {}, "from": {}, "full": {}, "function": {}, "goto": {}, "grant": {},
	"group": {}, "having": {}, "holdlock": {}, "identity": {}, "identity_insert": {}, "identitycol": {},
	"if": {}, "in": {}, "index": {}, "inner": {}, "insert": {}, "intersect": {}, "into": {},
	"is": {}, "join": {}, "key": {}, "kill": {}, "left": {}, "like": {}, "lineno": {},
	"load": {}, "merge": {}, "national": {}, "nocheck": {}, "nonclustered": {}, "not": {},
	"null": {}, "nullif": {}, "of": {}, "off": {}, "offsets": {}, "on": {}, "once": {},
	"only": {}, "open": {}, "opendatasource": {}, "openquery": {}, "openrowset": {}, "openxml": {},
	"option": {}, "or": {}, "order": {}, "outer": {}, "over": {}, "percent": {}, "pivot": {},
	"plan": {}, "precision": {}, "primary": {}, "print": {}, "proc": {}, "procedure": {},
	"public": {}, "raiserror": {}, "read": {}, "readtext": {}, "reconfigure": {}, "references": {},
	"replication": {}, "restore": {}, "restrict": {}, "return": {}, "revert": {}, "revoke": {},
	"right": {}, "rollback": {}, "rowcount": {}, "rowguidcol": {}, "rule": {}, "save": {},
	"schema": {}, "securityaudit": {}, "select": {}, "semantickeyphrasetable": {},
	"semanticsimilaritydetailstable": {}, "semanticsimilaritytable": {}, "session_user": {}, "set": {},
	"setuser": {}, "shutdown": {}, "some": {}, "statistics": {}, "system_user": {}, "table": {},
	"tablesample": {}, "textsize": {}, "then": {}, "to": {}, "top": {}, "tran": {}, "transaction": {},
	"trigger": {}, "truncate": {}, "try_convert": {}, "tsequal": {}, "union": {}, "unique": {},
	"unpivot": {}, "update": {}, "updatetext": {}, "use": {}, "user": {}, "values": {},
	"varying": {}, "view": {}, "waitfor": {}, "when": {}, "where": {}, "while": {}, "with": {},
	"within": {}, "writetext": {}, "postgres": {}, "root": {}, "admin": {}, "system": {}, "mysql": {},
}

func isReservedDatabaseWord(value string) bool {
	_, ok := reservedDatabaseWords[value]
	return ok
}

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

	if err := h.databaseService.LogAudit(log); err != nil {
		slog.Warn("Failed to record database audit log", "project_id", projectID, "error", err)
	}
}

// GetCredentials returns database credentials
func (h *DatabaseHandler) GetCredentials(c *fiber.Ctx) error {
	project, err := h.requireProjectWithDatabase(c)
	if err != nil {
		return err
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
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

	newPassword := utils.GeneratePassword(16)

	var lockedProject models.Project
	var jobID string
	var isRunning bool

	// Guard against concurrent operations using a pessimistic transaction lock
	errTx := h.db.Transaction(func(tx *gorm.DB) error {
		// Acquire pessimistic row-level lock on the project to prevent race-triggered corruption
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lockedProject, project.ID).Error; err != nil {
			return err
		}

		isQueued, errCheck := h.redisService.IsProjectQueued(lockedProject.ID)
		if errCheck == nil && isQueued {
			return fiber.ErrConflict
		}

		if lockedProject.DeploymentStatus != models.DepStatusCompleted &&
			lockedProject.DeploymentStatus != models.DepStatusFailed &&
			lockedProject.DeploymentStatus != models.DepStatusCancelled {
			return fiber.ErrConflict
		}

		// 1. Update password inside container engine
		if instance.Engine == "postgresql" {
			pgService := infrastructure.NewPostgreSQLService()
			if err := pgService.UpdatePassword(instance.Username, newPassword); err != nil {
				return err
			}
		} else {
			mysqlService := infrastructure.NewMySQLService()
			if err := mysqlService.UpdatePassword(instance.Username, newPassword); err != nil {
				return err
			}
		}

		// 2. Persist in database records within the atomic transaction
		instance.Password = newPassword
		if err := tx.Save(instance).Error; err != nil {
			return err
		}

		lockedProject.DatabasePassword = newPassword

		if err := tx.Save(&lockedProject).Error; err != nil {
			return err
		}

		return nil
	})

	if errTx != nil {
		h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "failed", errTx.Error())
		if errors.Is(errTx, fiber.ErrConflict) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Project has an active deployment or operation in progress. Please wait."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Credentials rotation failed: " + errTx.Error()})
	}

	isRunning = lockedProject.ContainerID != nil && *lockedProject.ContainerID != ""

	// 3. Trigger fan-out env propagation to OTHER projects bound to the same secret store
	h.secretStoreService.PropagateDatabaseEnvFanout(&lockedProject)

	if isRunning {
		var errQueue error
		jobID, errQueue = h.redisService.EnqueueDeployment(lockedProject.ID, lockedProject.UserID, "update_env")
		if errQueue != nil {
			// Compensate by saving failed deployment state and returning an error
			msg := fmt.Sprintf("Failed to queue environment update after rotating credentials: %v", errQueue)
			h.db.Model(&models.Project{}).Where("id = ?", lockedProject.ID).Updates(map[string]interface{}{
				"deployment_status":  models.DepStatusFailed,
				"deployment_message": msg,
			})
			h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "failed", errQueue.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to queue environment update: " + errQueue.Error()})
		}

		// Persist the job id and status atomically after successful enqueue
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

	if isRunning {
		return c.JSON(fiber.Map{
			"success": true,
			"message": "Database credentials rotated successfully. Environment update queued.",
			"job_id":  jobID,
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database credentials rotated successfully. Environment updated.",
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

	if req.Suspend {
		if instance.Engine == "postgresql" {
			pgService := infrastructure.NewPostgreSQLService()
			if err := pgService.UpdateStatus(instance.Name, instance.Username, true); err != nil {
				slog.Warn("Failed to suspend PostgreSQL database", "project_id", project.ID, "error", err.Error())
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to suspend PostgreSQL database"})
			}
		} else {
			mysqlService := infrastructure.NewMySQLService()
			if err := mysqlService.UpdateStatus(instance.Name, instance.Username, true); err != nil {
				slog.Warn("Failed to suspend MySQL database", "project_id", project.ID, "error", err.Error())
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to suspend MySQL database"})
			}
		}
		instance.Status = models.DBStatusSuspended
	} else {
		if instance.Engine == "postgresql" {
			pgService := infrastructure.NewPostgreSQLService()
			if err := pgService.UpdateStatus(instance.Name, instance.Username, false); err != nil {
				slog.Warn("Failed to resume PostgreSQL database", "project_id", project.ID, "error", err.Error())
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resume PostgreSQL database"})
			}
		} else {
			mysqlService := infrastructure.NewMySQLService()
			if err := mysqlService.UpdateStatus(instance.Name, instance.Username, false); err != nil {
				slog.Warn("Failed to resume MySQL database", "project_id", project.ID, "error", err.Error())
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resume MySQL database"})
			}
		}
		instance.Status = models.DBStatusActive
	}

	if err := h.db.Save(instance).Error; err != nil {
		slog.Warn("Failed to persist database status update", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to persist database status update"})
	}

	h.recordAuditLog(c, project.ID, "db_status_change", string(instance.Status), string(instance.Status), "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"status":  instance.Status,
		"message": fmt.Sprintf("Database instance status updated to %s.", instance.Status),
	})
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
		"allocated":    instance.StorageAllocation,
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
			// Exclude connections originating from the PaaS backend itself (using secure salted app name)
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
		"storage_limit":      instance.StorageAllocation,
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

	// 5. Fetch source database instance
	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(sourceProject.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned for source project"})
	}

	// 6. Check if target project already has an active database instance (Option 1: Prevent)
	targetInstance, err := h.databaseService.GetDatabaseInstanceByProjectID(targetProject.ID)
	if err == nil && targetInstance != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Target project already has an active database instance"})
	}

	// 7. Update database record associations
	oldProjectID := instance.ProjectID
	targetProjID := targetProject.ID
	instance.ProjectID = &targetProjID
	if err := h.db.Save(instance).Error; err != nil {
		slog.Warn("Failed to update database instance ownership", "project_id", sourceProject.ID, "target_project_id", targetProject.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update database instance ownership"})
	}

	// Also update Project database name & password fields to match!
	targetProject.DatabaseName = sourceProject.DatabaseName
	targetProject.DatabasePassword = sourceProject.DatabasePassword
	if err := h.db.Save(targetProject).Error; err != nil {
		slog.Warn("Failed to update target project database credentials", "error", err)
	}

	sourceProject.DatabaseName = nil
	sourceProject.DatabasePassword = ""
	if err := h.db.Save(sourceProject).Error; err != nil {
		slog.Warn("Failed to update source project database credentials", "error", err)
	}

	// 8. Record audit log
	oldProjIDStr := "nil"
	if oldProjectID != nil {
		oldProjIDStr = strconv.Itoa(int(*oldProjectID))
	}
	h.recordAuditLog(c, sourceProject.ID, "db_transfer", oldProjIDStr, strconv.Itoa(int(targetProject.ID)), "completed", "")

	// 9. Hot-swap environment binding: trigger env update redeployment for both source and target projects
	sourceJobID, err := h.redisService.EnqueueDeployment(sourceProject.ID, sourceProject.UserID, "update_env")
	if err != nil {
		slog.Warn("Failed to enqueue source project update_env deployment", "project_id", sourceProject.ID, "error", err)
	}

	targetJobID, err := h.redisService.EnqueueDeployment(targetProject.ID, targetProject.UserID, "update_env")
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

	// Fetch DatabaseInstance and validate ownership
	var dbInst models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
	}
	if dbInst.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}

	// Verify database is not already attached to a project
	if dbInst.ProjectID != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database is already attached to a project"})
	}

	// Fetch project by UID and validate ownership
	project, err := h.projectService.GetProjectByUID(req.ProjectUID)
	if err != nil || project == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}
	if project.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this project"})
	}

	// Verify project does not already have a database attached
	var count int64
	h.db.Model(&models.DatabaseInstance{}).Where("project_id = ?", project.ID).Count(&count)
	if count > 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Project already has a database attached"})
	}

	// Start a transaction to attach and sync cache
	errTx := h.db.Transaction(func(tx *gorm.DB) error {
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

		return nil
	})

	if errTx != nil {
		h.recordAuditLog(c, project.ID, "db_attach", "active", "active", "failed", errTx.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to attach database: " + errTx.Error()})
	}

	// Propagate env updates (after commit)
	_ = h.secretStoreService.PropagateDatabaseEnv(project)

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

	// Fetch DatabaseInstance and validate ownership
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

	// Fetch project
	var project models.Project
	if err := h.db.First(&project, projectID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	if project.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own the project associated with this database"})
	}

	// Start transaction to detach
	errTx := h.db.Transaction(func(tx *gorm.DB) error {
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

		return nil
	})

	if errTx != nil {
		h.recordAuditLog(c, projectID, "db_detach", "active", "active", "failed", errTx.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to detach database: " + errTx.Error()})
	}

	// Propagate env updates (removes DB_* vars since association is now nil, after commit)
	_ = h.secretStoreService.PropagateDatabaseEnv(&project)

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

	// Fetch DatabaseInstance and validate ownership
	var dbInst models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
	}
	if dbInst.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}

	newPassword := utils.GeneratePassword(16)

	// 1. Drop and recreate schema
	if dbInst.Engine == "postgresql" {
		pgService := infrastructure.NewPostgreSQLService()
		if err := pgService.DropDatabase(dbInst.Name); err != nil {
			slog.Error("Failed to drop postgresql database during reinstall", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to drop database: " + err.Error()})
		}
		if err := pgService.CreateDatabase(dbInst.Name, newPassword); err != nil {
			slog.Error("Failed to recreate postgresql database during reinstall", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to recreate database: " + err.Error()})
		}
	} else {
		mysqlService := infrastructure.NewMySQLService()
		if err := mysqlService.DropDatabase(dbInst.Name); err != nil {
			slog.Error("Failed to drop mysql database during reinstall", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to drop database: " + err.Error()})
		}
		if err := mysqlService.CreateDatabase(dbInst.Name, newPassword); err != nil {
			slog.Error("Failed to recreate mysql database during reinstall", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to recreate database: " + err.Error()})
		}
	}

	redeployRequired := false
	var project models.Project

	// 2. Persist in database records
	errTx := h.db.Transaction(func(tx *gorm.DB) error {
		dbInst.Password = newPassword
		if err := tx.Save(&dbInst).Error; err != nil {
			return err
		}

		if dbInst.ProjectID != nil {
			redeployRequired = true
			if err := tx.First(&project, *dbInst.ProjectID).Error; err != nil {
				return err
			}
			project.DatabasePassword = newPassword
			if err := tx.Save(&project).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if errTx != nil {
		h.recordAuditLog(c, 0, "db_reinstall", "active", "active", "failed", errTx.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reinstall database: " + errTx.Error()})
	}

	if redeployRequired {
		// Update credentials in secret store (after commit)
		_ = h.secretStoreService.PropagateDatabaseEnv(&project)
	}

	h.databaseService.InvalidateProjectDB(dbInst.Name)
	h.recordAuditLog(c, 0, "db_reinstall", "active", "active", "completed", "")
	return c.JSON(fiber.Map{
		"success":          true,
		"message":          "Database reinstalled and password rotated successfully",
		"redeployRequired": redeployRequired,
	})
}

// CreateDatabase request body definition
type CreateDatabaseRequest struct {
	Engine   string `json:"engine"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateDatabase handles creation of a standalone database instance
func (h *DatabaseHandler) CreateDatabase(c *fiber.Ctx) error {
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

	// Validate engine
	req.Engine = strings.ToLower(req.Engine)
	if req.Engine == "postgres" {
		req.Engine = "postgresql"
	}
	if req.Engine != "mysql" && req.Engine != "postgresql" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid database engine. Supported: mysql, postgresql"})
	}

	// Trim space if desired, and reject casing drift explicitly before mutation
	req.Name = strings.TrimSpace(req.Name)
	req.Username = strings.TrimSpace(req.Username)

	if req.Name != strings.ToLower(req.Name) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database name must be strictly lowercase"})
	}
	if req.Username != strings.ToLower(req.Username) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database username must be strictly lowercase"})
	}

	// Regex pattern validations (re-use pattern defined in PostgreSQL/MySQL services)
	// Spec:
	// - Database Name: min 2, max 64, alphanumeric/underscore, must NOT start with number or underscore (must be lowercase)
	// - Username: min 2, max 32, alphanumeric/underscore, MUST start with a letter (must be lowercase)
	// - Password: min 12, max 128, must contain at least one uppercase, one lowercase, and one number. No spaces or connection-string-breaking characters.
	var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	var userRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

	if !nameRegex.MatchString(req.Name) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database name must be 2-64 characters, start with a letter, and contain only alphanumeric characters or underscores"})
	}
	if !userRegex.MatchString(req.Username) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database username must be 2-32 characters, start with a letter, and contain only alphanumeric characters or underscores"})
	}

	// Password strength check
	if len(req.Password) < 12 || len(req.Password) > 128 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database password must be 12-128 characters long"})
	}
	// Check spaces or connection-string-breaking characters: e.g. space, double quote, single quote, backtick, backslash, semicolon, @, #, /, ?
	if strings.ContainsAny(req.Password, " \"'`\\;@#/?") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": databasePasswordInvalidCharsMessage})
	}
	hasUpper := false
	hasLower := false
	hasDigit := false
	for _, char := range req.Password {
		if char >= 'A' && char <= 'Z' {
			hasUpper = true
		} else if char >= 'a' && char <= 'z' {
			hasLower = true
		} else if char >= '0' && char <= '9' {
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Database password must contain at least one uppercase letter, one lowercase letter, and one number"})
	}

	if isReservedDatabaseWord(req.Name) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("Database name '%s' is a reserved SQL word", req.Name)})
	}
	if isReservedDatabaseWord(req.Username) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": fmt.Sprintf("Database username '%s' is a reserved SQL word", req.Username)})
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

	// Provision database in engine container
	if req.Engine == "postgresql" {
		pgService := infrastructure.NewPostgreSQLService()
		if err := pgService.CreateDatabaseCustom(req.Name, req.Username, req.Password); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to provision PostgreSQL database: " + err.Error()})
		}
	} else {
		mysqlService := infrastructure.NewMySQLService()
		if err := mysqlService.CreateDatabaseCustom(req.Name, req.Username, req.Password); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to provision MySQL database: " + err.Error()})
		}
	}

	// Create DatabaseInstance record
	dbInst := &models.DatabaseInstance{
		UserID:            userID,
		Engine:            req.Engine,
		Name:              req.Name,
		Username:          req.Username, // Use actual requested username instead of hardcoded req.Name
		Password:          req.Password,
		Host:              host,
		Port:              port,
		Status:            models.DBStatusActive,
		StorageAllocation: 1073741824, // 1GB -> TODO to make configurable by plan or user input in future
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := h.db.Create(dbInst).Error; err != nil {
		// Compensating cleanup on DB insert failure to prevent orphan physical DBs
		slog.Error("Failed to save database instance record. Initiating compensating cleanup.", "error", err.Error())
		if req.Engine == "postgresql" {
			pgService := infrastructure.NewPostgreSQLService()
			_ = pgService.DropDatabaseCustom(req.Name, req.Username)
		} else {
			mysqlService := infrastructure.NewMySQLService()
			_ = mysqlService.DropDatabaseCustom(req.Name, req.Username)
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to save database record: " + err.Error()})
	}

	h.recordAuditLog(c, 0, "db_create", "", "active", "completed", "")

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"success":  true,
		"database": dbInst,
	})
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

	// Fetch DatabaseInstance and validate ownership
	var dbInst models.DatabaseInstance
	if err := h.db.Where("uid = ?", dbUID).First(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database not found"})
	}
	if dbInst.UserID != userID {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "Forbidden: You do not own this database"})
	}

	// Verify database is not attached (ProjectID must be nil)
	if dbInst.ProjectID != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "Database is currently attached to a project. Detach it first."})
	}

	// Drop schema & user in engine container
	if dbInst.Engine == "postgresql" {
		pgService := infrastructure.NewPostgreSQLService()
		if err := pgService.DropDatabaseCustom(dbInst.Name, dbInst.Username); err != nil {
			slog.Error("Failed to drop PostgreSQL database", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to drop physical PostgreSQL database: " + err.Error()})
		}
	} else {
		mysqlService := infrastructure.NewMySQLService()
		if err := mysqlService.DropDatabaseCustom(dbInst.Name, dbInst.Username); err != nil {
			slog.Error("Failed to drop MySQL database", "db", dbInst.Name, "error", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to drop physical MySQL database: " + err.Error()})
		}
	}

	// Delete related backups (file + record)
	var backups []models.DatabaseBackup
	if err := h.db.Where("database_instance_id = ?", dbInst.ID).Find(&backups).Error; err == nil {
		for _, backup := range backups {
			// Delete backup files safely using validated paths and existing storage conventions
			if backup.Path != "" {
				// Prevent path traversal
				cleanedPath := filepath.Clean(backup.Path)
				// Resolve absolute path to check containment
				absPath, err := filepath.Abs(cleanedPath)
				if err != nil {
					slog.Error("Failed to resolve absolute path of backup file", "path", backup.Path, "error", err.Error())
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resolve backup file path: " + err.Error()})
				}

				absDataPath, err := filepath.Abs(h.cfg.DataPath)
				if err != nil {
					slog.Error("Failed to resolve absolute data path", "error", err.Error())
					return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resolve storage data path: " + err.Error()})
				}

				// The backup file must reside inside h.cfg.DataPath to prevent arbitrary file deletion (such as /etc or root dirs)
				rel, err := filepath.Rel(absDataPath, absPath)
				if err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
					if err := os.Remove(absPath); err != nil && !os.IsNotExist(err) {
						slog.Error("Failed to delete physical database backup file", "path", absPath, "error", err.Error())
						return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete backup file: " + err.Error()})
					}
				} else {
					slog.Error("Backup file path is outside the allowed storage directory", "path", absPath, "allowed", absDataPath)
					return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Security violation: Backup path is outside the allowed storage directory"})
				}
			}
			if err := h.db.Delete(&backup).Error; err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete backup record: " + err.Error()})
			}
		}
	}

	// Soft delete the DatabaseInstance record: Set status = "deleted"
	dbInst.Status = models.DBStatusDeleted
	dbInst.ProjectID = nil // explicitly clear just in case
	if err := h.db.Save(&dbInst).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update database status: " + err.Error()})
	}

	h.recordAuditLog(c, 0, "db_delete", "active", "deleted", "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database deleted successfully",
	})
}
