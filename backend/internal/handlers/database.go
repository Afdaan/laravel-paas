// ===========================================
// Database Manager Handler
// ===========================================
// Allows students to manage their project databases
// ===========================================
package handlers

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
	"gorm.io/gorm"

	_ "github.com/go-sql-driver/mysql"
)

// DatabaseHandler handles database management endpoints
type DatabaseHandler struct {
	db  *gorm.DB
	cfg *config.Config
}

// NewDatabaseHandler creates a new database handler
func NewDatabaseHandler(db *gorm.DB, cfg *config.Config) *DatabaseHandler {
	return &DatabaseHandler{db: db, cfg: cfg}
}

// TableInfo represents table metadata
type TableInfo struct {
	Name    string `json:"name"`
	Rows    int64  `json:"rows"`
	Size    string `json:"size"`
	Engine  string `json:"engine"`
	Created string `json:"created"`
}

// ColumnInfo represents column metadata
type ColumnInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Key      string  `json:"key"`
	Default  *string `json:"default"`
	Extra    string  `json:"extra"`
}

// QueryResult represents SQL query result
type QueryResult struct {
	Columns      []string                 `json:"columns"`
	Rows         []map[string]interface{} `json:"rows"`
	RowsAffected int64                    `json:"rows_affected"`
	Duration     string                   `json:"duration"`
}

// connectToProjectDB connects to a student's project database
func (h *DatabaseHandler) connectToProjectDB(project *models.Project) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(paas-mysql:3306)/%s?parseTime=true",
		project.DatabaseName,
		project.DatabaseName,
		project.DatabaseName,
	)
	return sql.Open("mysql", dsn)
}

// getProjectForUser fetches project and validates ownership
func (h *DatabaseHandler) getProjectForUser(c *fiber.Ctx) (*models.Project, error) {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid project ID")
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)

	var project models.Project
	query := h.db

	// Students can only access their own projects
	if role == string(models.RoleStudent) {
		query = query.Where("user_id = ?", userID)
	}

	if err := query.First(&project, id).Error; err != nil {
		return nil, fmt.Errorf("project not found")
	}

	return &project, nil
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

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect to database"})
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT 
			TABLE_NAME,
			TABLE_ROWS,
			ROUND(((DATA_LENGTH + INDEX_LENGTH) / 1024), 2) AS size_kb,
			ENGINE,
			CREATE_TIME
		FROM information_schema.TABLES 
		WHERE TABLE_SCHEMA = ?
		ORDER BY TABLE_NAME
	`, project.DatabaseName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		var sizeKB float64
		var created sql.NullTime
		if err := rows.Scan(&t.Name, &t.Rows, &sizeKB, &t.Engine, &created); err != nil {
			continue
		}
		t.Size = fmt.Sprintf("%.2f KB", sizeKB)
		if created.Valid {
			t.Created = created.Time.Format("2006-01-02 15:04")
		}
		tables = append(tables, t)
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

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect"})
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT 
			COLUMN_NAME,
			COLUMN_TYPE,
			IS_NULLABLE,
			COLUMN_KEY,
			COLUMN_DEFAULT,
			EXTRA
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`, project.DatabaseName, tableName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var nullable string
		if err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Key, &col.Default, &col.Extra); err != nil {
			continue
		}
		col.Nullable = nullable == "YES"
		columns = append(columns, col)
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
	offset := (page - 1) * limit

	// Validate table name (prevent SQL injection)
	if !isValidIdentifier(tableName) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid table name"})
	}

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect"})
	}
	defer db.Close()

	// Get total count
	var total int64
	db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)).Scan(&total)

	// Get data
	query := fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d", tableName, limit, offset)
	rows, err := db.Query(query)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	defer rows.Close()

	columns, _ := rows.Columns()
	var data []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		rows.Scan(valuePtrs...)

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		data = append(data, row)
	}

	return c.JSON(fiber.Map{
		"columns": columns,
		"rows":    data,
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

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Query is required"})
	}

	// Block dangerous operations
	upperQuery := strings.ToUpper(query)
	if strings.Contains(upperQuery, "DROP DATABASE") ||
		strings.Contains(upperQuery, "CREATE DATABASE") ||
		strings.Contains(upperQuery, "GRANT") ||
		strings.Contains(upperQuery, "REVOKE") {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "This operation is not allowed"})
	}

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect"})
	}
	defer db.Close()

	start := time.Now()

	// Check if it's a SELECT query
	if strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "SHOW") || strings.HasPrefix(upperQuery, "DESCRIBE") {
		rows, err := db.Query(query)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()

		columns, _ := rows.Columns()
		var data []map[string]interface{}

		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			rows.Scan(valuePtrs...)

			row := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					row[col] = string(b)
				} else {
					row[col] = val
				}
			}
			data = append(data, row)
		}

		return c.JSON(QueryResult{
			Columns:  columns,
			Rows:     data,
			Duration: time.Since(start).String(),
		})
	}

	// Execute non-SELECT query
	result, err := db.Exec(query)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	affected, _ := result.RowsAffected()
	return c.JSON(QueryResult{
		RowsAffected: affected,
		Duration:     time.Since(start).String(),
	})
}

// ExportDatabase exports database as SQL
func (h *DatabaseHandler) ExportDatabase(c *fiber.Ctx) error {
	project, err := h.getProjectForUser(c)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
	}

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect"})
	}
	defer db.Close()

	var sqlDump strings.Builder
	sqlDump.WriteString(fmt.Sprintf("-- Database Export: %s\n", project.DatabaseName))
	sqlDump.WriteString(fmt.Sprintf("-- Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	// Get all tables
	tables, _ := db.Query("SHOW TABLES")
	defer tables.Close()

	for tables.Next() {
		var tableName string
		tables.Scan(&tableName)

		// Get CREATE TABLE statement
		var tbl, createStmt string
		db.QueryRow(fmt.Sprintf("SHOW CREATE TABLE `%s`", tableName)).Scan(&tbl, &createStmt)
		sqlDump.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))
		sqlDump.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS `%s`;\n", tableName))
		sqlDump.WriteString(createStmt + ";\n\n")

		// Get table data
		rows, err := db.Query(fmt.Sprintf("SELECT * FROM `%s`", tableName))
		if err != nil {
			continue
		}

		columns, _ := rows.Columns()
		if len(columns) == 0 {
			rows.Close()
			continue
		}

		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			rows.Scan(valuePtrs...)

			var vals []string
			for _, v := range values {
				if v == nil {
					vals = append(vals, "NULL")
				} else if b, ok := v.([]byte); ok {
					vals = append(vals, fmt.Sprintf("'%s'", escapeSQLString(string(b))))
				} else {
					vals = append(vals, fmt.Sprintf("'%v'", v))
				}
			}
			sqlDump.WriteString(fmt.Sprintf("INSERT INTO `%s` VALUES (%s);\n", tableName, strings.Join(vals, ", ")))
		}
		rows.Close()
		sqlDump.WriteString("\n")
	}

	c.Set("Content-Type", "application/sql")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s_%s.sql", project.DatabaseName, time.Now().Format("20060102_150405")))
	return c.SendString(sqlDump.String())
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

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect"})
	}
	defer db.Close()

	// Split by semicolons and execute each statement
	statements := strings.Split(req.SQL, ";")
	successCount := 0
	var errors []string

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}

		// Block dangerous operations
		upperStmt := strings.ToUpper(stmt)
		if strings.Contains(upperStmt, "DROP DATABASE") ||
			strings.Contains(upperStmt, "CREATE DATABASE") {
			errors = append(errors, "Blocked: DROP/CREATE DATABASE not allowed")
			continue
		}

		if _, err := db.Exec(stmt); err != nil {
			errors = append(errors, fmt.Sprintf("Error: %s", err.Error()))
		} else {
			successCount++
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

	db, err := h.connectToProjectDB(project)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to connect"})
	}
	defer db.Close()

	// Get all tables
	rows, _ := db.Query("SHOW TABLES")
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		rows.Scan(&table)
		tables = append(tables, table)
	}

	// Disable foreign key checks
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")

	// Drop all tables
	for _, table := range tables {
		db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	}

	// Re-enable foreign key checks
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	return c.JSON(fiber.Map{
		"success": true,
		"dropped": len(tables),
	})
}

// Helper functions
func isValidIdentifier(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return len(s) > 0 && len(s) < 64
}

func escapeSQLString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}
