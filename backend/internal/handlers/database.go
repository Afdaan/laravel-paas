// ===========================================
// Database Manager Handler
// ===========================================
// Allows users to manage their project databases
// ===========================================
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
	"github.com/laravel-paas/backend/internal/services"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"gorm.io/gorm"
)

// DatabaseHandler handles database management endpoints
type DatabaseHandler struct {
	db              *gorm.DB
	cfg             *config.Config
	databaseService *services.DatabaseService
	projectService  *projectServicePkg.ProjectService
	redisService    *infrastructure.RedisService
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(
	db *gorm.DB,
	cfg *config.Config,
	databaseService *services.DatabaseService,
	projectService *projectServicePkg.ProjectService,
	redisService *infrastructure.RedisService,
) *DatabaseHandler {
	return &DatabaseHandler{
		db:              db,
		cfg:             cfg,
		databaseService: databaseService,
		projectService:  projectService,
		redisService:    redisService,
	}
}

// getProjectForUser fetches project and validates ownership
func (h *DatabaseHandler) getProjectForUser(c *fiber.Ctx) (*models.Project, error) {
	idParam := c.Params("id")

	var project *models.Project
	var err error

	// 1. Try to fetch by UID column first (Standard for frontend)
	project, err = h.projectService.GetProjectByUID(idParam)
	if err != nil {
		// 2. Fallback: Check if it's a numeric ID (for transition/admin)
		id, errConv := strconv.Atoi(idParam)
		if errConv == nil {
			project, err = h.projectService.GetProjectByID(uint(id))
		}
	}

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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		// Fallback to legacy MySQL details if DatabaseInstance not initialized yet
		return c.JSON(fiber.Map{
			"engine":   "mysql",
			"host":     "paas-mysql",
			"port":     3306,
			"database": project.DatabaseName,
			"username": project.DatabaseName,
			"password": project.DatabasePassword,
		})
	}

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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned for this project"})
	}

	newPassword := utils.GeneratePassword(16)

	// 1. Update password inside container engine
	if instance.Engine == "postgresql" {
		pgService := infrastructure.NewPostgreSQLService()
		if err := pgService.UpdatePassword(instance.Name, newPassword); err != nil {
			h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "failed", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update PostgreSQL database password: " + err.Error()})
		}
	} else {
		mysqlService := infrastructure.NewMySQLService()
		if err := mysqlService.UpdatePassword(instance.Name, newPassword); err != nil {
			h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "failed", err.Error())
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update MySQL database password: " + err.Error()})
		}
	}

	// 2. Persist in database records
	instance.Password = newPassword
	project.DatabasePassword = newPassword

	if err := h.db.Save(project).Error; err != nil {
		slog.Warn("Failed to update project password in GORM", "error", err)
	}

	if err := h.db.Save(instance).Error; err != nil {
		slog.Warn("Failed to update database instance password in GORM", "error", err)
	}

	// 3. Trigger Environment hot-swap redeployment job in Redis
	jobID, err := h.redisService.EnqueueDeployment(project.ID, project.UserID, "update_env")
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "partially_completed", "Failed to queue update_env: "+err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Credentials updated, but failed to queue zero-downtime hot-swap: " + err.Error()})
	}

	h.recordAuditLog(c, project.ID, "db_credential_rotate", "active", "active", "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database credentials rotated successfully. Zero-downtime environment update queued.",
		"job_id":  jobID,
	})
}

// RestartDatabase resets connection pool and tests ping
func (h *DatabaseHandler) RestartDatabase(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	// Test connection ping
	db, err := h.databaseService.ConnectToProjectDB(instance.Name, instance.Password)
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_restart", "active", "active", "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database is unreachable: " + err.Error()})
	}

	if err := db.Ping(); err != nil {
		h.recordAuditLog(c, project.ID, "db_restart", "active", "active", "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Database connection is unhealthy: " + err.Error()})
	}

	h.recordAuditLog(c, project.ID, "db_restart", "active", "active", "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database connection pool refreshed and verified healthy.",
	})
}

// UpdateStatus suspends or resumes database instance
func (h *DatabaseHandler) UpdateStatus(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
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
			if err := pgService.UpdateStatus(instance.Name, true); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to suspend PostgreSQL database: " + err.Error()})
			}
		} else {
			mysqlService := infrastructure.NewMySQLService()
			if err := mysqlService.UpdateStatus(instance.Name, true); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to suspend MySQL database: " + err.Error()})
			}
		}
		instance.Status = models.DBStatusSuspended
	} else {
		if instance.Engine == "postgresql" {
			pgService := infrastructure.NewPostgreSQLService()
			if err := pgService.UpdateStatus(instance.Name, false); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resume PostgreSQL database: " + err.Error()})
			}
		} else {
			mysqlService := infrastructure.NewMySQLService()
			if err := mysqlService.UpdateStatus(instance.Name, false); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resume MySQL database: " + err.Error()})
			}
		}
		instance.Status = models.DBStatusActive
	}

	if err := h.db.Save(instance).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to persist database status update in GORM: " + err.Error()})
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance.Name, instance.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect to database: " + err.Error()})
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance.Name, instance.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve tables: " + err.Error()})
	}

	type TableSchema struct {
		Name    string               `json:"name"`
		Rows    int64                `json:"rows"`
		Columns []services.ColumnInfo `json:"columns"`
	}

	var schemas []TableSchema
	for _, t := range tables {
		cols, err := h.databaseService.GetTableStructure(instance.Name, instance.Password, t.Name)
		if err == nil {
			schemas = append(schemas, TableSchema{
				Name:    t.Name,
				Rows:    t.Rows,
				Columns: cols,
			})
		}
	}

	return c.JSON(fiber.Map{
		"tables": schemas,
	})
}

// ExecuteDesignerAction handles graphical GUI designer modifications and writes audits
func (h *DatabaseHandler) ExecuteDesignerAction(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	var req services.DesignerActionRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request payload"})
	}

	err = h.databaseService.ExecuteDesignerAction(instance.Name, instance.Password, req)
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_designer_"+req.Action, req.TableName, req.NewName, "failed", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	backup, err := h.databaseService.CreateBackup(project.ID)
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_backup_create", "", "", "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	backupID, err := strconv.Atoi(c.Params("backup"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup ID"})
	}

	err = h.databaseService.RestoreBackup(project.ID, uint(backupID))
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_backup_restore", "", strconv.Itoa(backupID), "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Restore failed: " + err.Error()})
	}

	h.recordAuditLog(c, project.ID, "db_backup_restore", "", strconv.Itoa(backupID), "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database successfully rolled back and restored to snapshot state.",
	})
}

// DeleteBackup prunes a database snapshot logically and physically
func (h *DatabaseHandler) DeleteBackup(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	backupID, err := strconv.Atoi(c.Params("backup"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid backup ID"})
	}

	err = h.databaseService.DeleteBackup(project.ID, uint(backupID))
	if err != nil {
		h.recordAuditLog(c, project.ID, "db_backup_delete", "", strconv.Itoa(backupID), "failed", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Prune failed: " + err.Error()})
	}

	h.recordAuditLog(c, project.ID, "db_backup_delete", "", strconv.Itoa(backupID), "completed", "")

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Database backup catalog record and snapshot file pruned permanently.",
	})
}

// GetMetrics returns active metrics like connections and size
func (h *DatabaseHandler) GetMetrics(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	instance, err := h.databaseService.GetDatabaseInstanceByProjectID(project.ID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Database instance not provisioned"})
	}

	tables, err := h.databaseService.ListProjectTables(instance.Name, instance.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	var totalSizeKB float64
	for _, t := range tables {
		var sz float64
		_, _ = fmt.Sscanf(t.Size, "%f KB", &sz)
		totalSizeKB += sz
	}

	// Enforce dynamic active connections check in the engine container
	db, err := h.databaseService.ConnectToProjectDB(instance.Name, instance.Password)
	activeConnections := 0
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if instance.Engine == "postgresql" {
			_ = db.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname = $1", instance.Name).Scan(&activeConnections)
		} else {
			_ = db.QueryRowContext(ctx, "SELECT count(*) FROM information_schema.processlist WHERE db = ?", instance.Name).Scan(&activeConnections)
		}
	}

	return c.JSON(fiber.Map{
		"active_connections": activeConnections,
		"size_kb":            totalSizeKB,
		"storage_limit":      instance.StorageAllocation,
	})
}

// ListTables returns all tables in the database (Legacy/Fallback)
func (h *DatabaseHandler) ListTables(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	tables, err := h.databaseService.ListProjectTables(project.DatabaseName, project.DatabasePassword)
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	tableName := c.Params("table")
	if tableName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Table name required"})
	}

	columns, err := h.databaseService.GetTableStructure(project.DatabaseName, project.DatabasePassword, tableName)
	if err != nil {
		slog.Warn("Failed to get table structure", "project_id", project.ID, "table", tableName, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve table structure"})
	}

	return c.JSON(fiber.Map{"columns": columns})
}

// GetTableData returns rows from a table with pagination (Legacy/Fallback)
func (h *DatabaseHandler) GetTableData(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	tableName := c.Params("table")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	columns, rows, total, err := h.databaseService.GetTableData(project.DatabaseName, project.DatabasePassword, tableName, page, limit)
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
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

	deleted, err := h.databaseService.DeleteTableRow(project.DatabaseName, project.DatabasePassword, tableName, req.PrimaryKey, req.Value)
	if err != nil {
		slog.Warn("Failed to delete table row", "project_id", project.ID, "table", tableName, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to delete row"})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"deleted": deleted,
	})
}

// ExecuteQuery runs a manual raw query (Legacy/Fallback)
func (h *DatabaseHandler) ExecuteQuery(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req ExecuteQueryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	result, err := h.databaseService.ExecuteRawQuery(project.DatabaseName, project.DatabasePassword, req.Query)
	if err != nil {
		appErr, ok := err.(*apperr.AppError)
		if ok && appErr != nil {
			return c.Status(appErr.HTTPStatus).JSON(fiber.Map{
				"error": appErr.Message,
				"code":  appErr.Code,
			})
		}
		slog.Error("Database query execution failed", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// ExportDatabase exports database as SQL (Legacy/Fallback)
func (h *DatabaseHandler) ExportDatabase(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	dump, err := h.databaseService.GenerateProjectDump(project.DatabaseName, project.DatabasePassword)
	if err != nil {
		slog.Warn("Failed to export database", "project_id", project.ID, "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to export database"})
	}

	c.Set("Content-Type", "application/sql")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.sql", project.DatabaseName))
	return c.SendString(dump)
}

// ImportDatabase imports SQL file (Legacy/Fallback)
func (h *DatabaseHandler) ImportDatabase(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	var req ImportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.SQL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SQL content is required"})
	}

	statements := strings.Split(req.SQL, ";")
	successCount := 0
	var errors []string

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		res, err := h.databaseService.ExecuteRawQuery(project.DatabaseName, project.DatabasePassword, stmt)
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
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	dropped, err := h.databaseService.ResetProjectDatabase(project.DatabaseName, project.DatabasePassword)
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
