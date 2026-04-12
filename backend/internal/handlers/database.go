// ===========================================
// Database Manager Handler
// ===========================================
// Allows students to manage their project databases
// ===========================================
package handlers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
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
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return nil, fmt.Errorf("invalid project ID")
	}

	userID := c.Locals("user_id").(uint)
	role := models.Role(c.Locals("role").(string))

	project, err := h.projectService.GetProjectByID(uint(id))
	if err != nil {
		return nil, fmt.Errorf("project not found")
	}

	if role != models.RoleAdmin && role != models.RoleSuperAdmin && project.UserID != userID {
		return nil, fmt.Errorf("project not found")
	}

	return project, nil
}

// GetCredentials returns database credentials
func (h *DatabaseHandler) GetCredentials(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"host":     "paas-mysql",
		"port":     3306,
		"database": project.DatabaseName,
		"username": project.DatabaseName,
		"password": project.DatabaseName,
	})
}

// ListTables returns all tables in the database
func (h *DatabaseHandler) ListTables(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	tables, err := h.databaseService.ListProjectTables(project.DatabaseName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"tables": tables})
}

// GetTableStructure returns columns for a table
func (h *DatabaseHandler) GetTableStructure(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	tableName := c.Params("table")
	if tableName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Table name required"})
	}

	columns, err := h.databaseService.GetTableStructure(project.DatabaseName, tableName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"columns": columns})
}

// GetTableData returns rows from a table with pagination
func (h *DatabaseHandler) GetTableData(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	tableName := c.Params("table")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	limit, _ := strconv.Atoi(c.Query("limit", "50"))

	columns, rows, total, err := h.databaseService.GetTableData(project.DatabaseName, tableName, page, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"columns": columns,
		"rows":    rows,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

// ExecuteQuery runs a SQL query
type ExecuteQueryRequest struct {
	Query string `json:"query"`
}

func (h *DatabaseHandler) ExecuteQuery(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var req ExecuteQueryRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	result, err := h.databaseService.ExecuteRawQuery(project.DatabaseName, req.Query)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(result)
}

// ExportDatabase exports database as SQL
func (h *DatabaseHandler) ExportDatabase(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	dump, err := h.databaseService.GenerateProjectDump(project.DatabaseName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	var req ImportRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.SQL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "SQL content is required"})
	}

	// This logic remains slightly in handler to handle the multifile/multistatement response structure if needed,
	// but let's shift it to service too for consistency.
	statements := strings.Split(req.SQL, ";")
	successCount := 0
	var errors []string

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		res, err := h.databaseService.ExecuteRawQuery(project.DatabaseName, stmt)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Error in statement: %s", err.Error()))
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	dropped, err := h.databaseService.ResetProjectDatabase(project.DatabaseName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"dropped": dropped,
	})
}
