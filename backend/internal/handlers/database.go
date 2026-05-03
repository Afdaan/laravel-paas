// ===========================================
// Database Manager Handler
// ===========================================
// Allows students to manage their project databases
// ===========================================
package handlers

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
)

// DatabaseHandler handles database management endpoints
type DatabaseHandler struct {
	cfg             *config.Config
	databaseService *services.DatabaseService
	projectService  *services.ProjectService
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(cfg *config.Config, databaseService *services.DatabaseService, projectService *services.ProjectService) *DatabaseHandler {
	return &DatabaseHandler{
		cfg:             cfg,
		databaseService: databaseService,
		projectService:  projectService,
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

// GetCredentials returns database credentials
func (h *DatabaseHandler) GetCredentials(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Project not found"})
	}

	return c.JSON(fiber.Map{
		"host":     "paas-mysql",
		"port":     3306,
		"database": project.DatabaseName,
		"username": project.DatabaseName,
		"password": project.DatabasePassword,
	})
}

// ListTables returns all tables in the database
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

// GetTableStructure returns columns for a table
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

// GetTableData returns rows from a table with pagination
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

// DeleteTableRowRequest represents the payload for deleting a row
type DeleteTableRowRequest struct {
	PrimaryKey string      `json:"primary_key"`
	Value      interface{} `json:"value"`
}

// DeleteTableRow deletes a specific row from a table
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

// ExecuteQueryRequest represents a SQL query execution request
type ExecuteQueryRequest struct {
	Query string `json:"query"`
}

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
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Query execution failed"})
	}

	return c.JSON(result)
}

// ExportDatabase exports database as SQL
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

// ImportDatabase imports SQL file
type ImportRequest struct {
	SQL string `json:"sql"`
}

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

// ResetDatabase drops all tables
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

// AdminListAll returns all student databases (Admin only)
func (h *DatabaseHandler) AdminListAll(c *fiber.Ctx) error {
	databases, err := h.databaseService.AdminListAllDatabases()
	if err != nil {
		slog.Error("Failed to list all databases", "error", err.Error())
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to retrieve database list"})
	}

	return c.JSON(fiber.Map{"databases": databases})
}
