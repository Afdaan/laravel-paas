// ===========================================
// Database Service
// ===========================================
// Orchestrates project database operations
// ===========================================
package services

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/config"
	"gorm.io/gorm"

	_ "github.com/go-sql-driver/mysql"
)

type DatabaseService struct {
	db   *gorm.DB
	cfg  *config.Config
	pool sync.Map // map[dbName]*sql.DB — cached connections per student database
}

func NewDatabaseService(db *gorm.DB, cfg *config.Config) *DatabaseService {
	return &DatabaseService{db: db, cfg: cfg}
}

type TableInfo struct {
	Name    string `json:"name"`
	Rows    int64  `json:"rows"`
	Size    string `json:"size"`
	Engine  string `json:"engine"`
	Created string `json:"created"`
}

type ColumnInfo struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Key      string  `json:"key"`
	Default  *string `json:"default"`
	Extra    string  `json:"extra"`
}

type QueryResult struct {
	Columns      []string                 `json:"columns"`
	Rows         []map[string]interface{} `json:"rows"`
	RowsAffected int64                    `json:"rows_affected"`
	Duration     string                   `json:"duration"`
}

type AdminDatabaseInfo struct {
	ProjectID    uint   `json:"project_id"`
	ProjectName  string `json:"project_name"`
	StudentName  string `json:"student_name"`
	DatabaseName string `json:"database_name"`
	TableCount   int    `json:"table_count"`
	Size         string `json:"size"`
	Status       string `json:"status"`
}

// ConnectToProjectDB returns a pooled connection to a student's MySQL database.
// Connections are cached and reused across requests to avoid connection storms.
func (s *DatabaseService) ConnectToProjectDB(dbName, password string) (*sql.DB, error) {
	// Return cached connection if available and healthy
	if cached, ok := s.pool.Load(dbName); ok {
		db := cached.(*sql.DB)
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Stale connection: remove and recreate
		s.pool.Delete(dbName)
		db.Close()
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		dbName, password, s.cfg.MYSQLHost, s.cfg.MYSQLPort, dbName,
	)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}

	// Conservative pool limits per student database
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	s.pool.Store(dbName, db)
	return db, nil
}

// ListProjectTables returns metadata for all tables in a project database
func (s *DatabaseService) ListProjectTables(dbName, password string) ([]TableInfo, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, err
	}

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
	`, dbName)
	if err != nil {
		return nil, err
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

	return tables, nil
}

// GetTableStructure returns column metadata for a specific table
func (s *DatabaseService) GetTableStructure(dbName, password, tableName string) ([]ColumnInfo, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, err
	}

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
	`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var nullable string
		if err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Key, &col.Default, &col.Extra); err != nil {
			continue
		}
		col.Nullable = (nullable == "YES")
		columns = append(columns, col)
	}

	return columns, nil
}

// GetTableData supports paginated data retrieval from a table
func (s *DatabaseService) GetTableData(dbName, password, tableName string, page, limit int) ([]string, []map[string]interface{}, int64, error) {
	if !s.isValidIdentifier(tableName) {
		return nil, nil, 0, apperr.New(400, "INVALID_TABLE_NAME", "Table name contains invalid characters or exceeds length limit")
	}

	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, nil, 0, err
	}

	var total int64
	if err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)).Scan(&total); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to count rows: %w", err)
	}

	// Enforce maximum safety limit
	if limit > 200 {
		limit = 200
	}

	offset := (page - 1) * limit
	query := fmt.Sprintf("SELECT * FROM `%s` LIMIT %d OFFSET %d", tableName, limit, offset)
	rows, err := db.Query(query)
	if err != nil {
		return nil, nil, 0, err
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

		if err := rows.Scan(valuePtrs...); err != nil {
			slog.Warn("Failed to scan row", "tableName", tableName, "error", err)
			continue
		}

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

	return columns, data, total, nil
}

// DeleteTableRow safely deletes a specific row from a table using a primary key
func (s *DatabaseService) DeleteTableRow(dbName, password, tableName, pkColumn string, pkValue interface{}) (int64, error) {
	if !s.isValidIdentifier(tableName) {
		return 0, apperr.New(400, "INVALID_TABLE_NAME", "Table name contains invalid characters")
	}
	if !s.isValidIdentifier(pkColumn) {
		return 0, apperr.New(400, "INVALID_COLUMN_NAME", "Column name contains invalid characters")
	}

	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return 0, err
	}

	query := fmt.Sprintf("DELETE FROM `%s` WHERE `%s` = ? LIMIT 1", tableName, pkColumn)

	result, err := db.Exec(query, pkValue)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

// ExecuteRawQuery runs a manual SQL query against a project database
func (s *DatabaseService) ExecuteRawQuery(dbName, password, query string) (*QueryResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, apperr.New(400, "EMPTY_QUERY", "Query cannot be empty")
	}

	upperQuery := strings.ToUpper(query)

	// Block dangerous operations
	blockedPatterns := []string{
		"DROP DATABASE",
		"CREATE DATABASE",
		"ALTER DATABASE",
		"GRANT",
		"REVOKE",
		"CREATE USER",
		"DROP USER",
		"ALTER USER",
		"SET PASSWORD",
		"FLUSH PRIVILEGES",
		"FLUSH HOSTS",
		"FLUSH LOGS",
		"LOAD_FILE",
		"INTO OUTFILE",
		"INTO DUMPFILE",
		"LOAD DATA",
		"SYSTEM ",
		"\\! ",
		"EXEC ",
		"EXECUTE ",
		"xp_",
		"INFORMATION_SCHEMA.PROCESSLIST",
		"mysql.",
		"performance_schema.",
		"sys.",
		"UNION SELECT",
		"SELECT.*INTO",
	}

	for _, pattern := range blockedPatterns {
		if strings.Contains(upperQuery, pattern) {
			slog.Warn("Blocked forbidden SQL operation attempt",
				"query", query[:min(len(query), 100)],
				"pattern", pattern,
			)
			return nil, apperr.New(403, "SQL_OPERATION_FORBIDDEN", "This SQL operation is not permitted for security reasons")
		}
	}

	// Only allow SELECT, SHOW, DESCRIBE, INSERT, UPDATE, DELETE on user's own database
	allowedPrefixes := []string{"SELECT", "SHOW", "DESCRIBE", "DESC", "INSERT", "UPDATE", "DELETE"}
	allowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(upperQuery, prefix) {
			allowed = true
			break
		}
	}

	if !allowed {
		return nil, apperr.New(403, "SQL_OPERATION_FORBIDDEN", "Only SELECT, INSERT, UPDATE, DELETE, SHOW, and DESCRIBE are allowed")
	}

	// Block cross-database queries
	if strings.Contains(query, ".") && !strings.Contains(upperQuery, "INFORMATION_SCHEMA") {
		parts := strings.Split(query, ".")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "`") {
				part = strings.Trim(part, "`")
			}
			if part != dbName && part != "information_schema" && len(part) > 0 && !strings.HasPrefix(strings.ToUpper(part), "SELECT") {
				if strings.Contains(upperQuery, "`"+part+"`") || strings.Contains(upperQuery, part+".") {
					slog.Warn("Blocked cross-database query attempt",
						"query", query[:min(len(query), 100)],
						"target_db", part,
					)
					return nil, apperr.New(403, "SQL_CROSS_DATABASE", "Cross-database queries are not allowed")
				}
			}
		}
	}

	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		slog.Error("Failed to connect to project database", "database", dbName, "error", err.Error())
		return nil, apperr.New(500, "DB_CONNECTION_FAILED", "Unable to connect to database")
	}

	start := time.Now()

	// Handle data retrieval queries
	if strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "SHOW") || strings.HasPrefix(upperQuery, "DESCRIBE") || strings.HasPrefix(upperQuery, "DESC") {
		rows, err := db.Query(query)
		if err != nil {
			slog.Warn("SQL query execution failed", "query", query[:min(len(query), 100)], "error", err.Error())
			return nil, apperr.New(400, "QUERY_ERROR", "Query execution failed")
		}
		defer rows.Close()

		columns, _ := rows.Columns()
		var data []map[string]interface{}
		rowCount := 0
		maxRows := 2000 // Safety cap to prevent OOM on large SELECTs

		for rows.Next() {
			if rowCount >= maxRows {
				break
			}
			rowCount++

			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				slog.Warn("Failed to scan row in raw query", "error", err)
				continue
			}

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

		return &QueryResult{
			Columns:  columns,
			Rows:     data,
			Duration: time.Since(start).String(),
		}, nil
	}

	// Handle modification queries
	result, err := db.Exec(query)
	if err != nil {
		slog.Warn("SQL modification query failed", "query", query[:min(len(query), 100)], "error", err.Error())
		return nil, apperr.New(400, "QUERY_ERROR", "Query execution failed")
	}

	affected, _ := result.RowsAffected()
	return &QueryResult{
		RowsAffected: affected,
		Duration:     time.Since(start).String(),
	}, nil
}

// GenerateProjectDump creates a logical SQL export for a project database
func (s *DatabaseService) GenerateProjectDump(dbName, password string) (string, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return "", err
	}

	var sqlDump strings.Builder
	sqlDump.WriteString(fmt.Sprintf("-- Database Export: %s\n", dbName))
	sqlDump.WriteString(fmt.Sprintf("-- Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	tables, _ := db.Query("SHOW TABLES")
	defer tables.Close()

	for tables.Next() {
		var tableName string
		if err := tables.Scan(&tableName); err != nil {
			continue
		}

		var tbl, createStmt string
		if err := db.QueryRow(fmt.Sprintf("SHOW CREATE TABLE `%s`", tableName)).Scan(&tbl, &createStmt); err != nil {
			continue
		}
		sqlDump.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))
		sqlDump.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS `%s`;\n", tableName))
		sqlDump.WriteString(createStmt + ";\n\n")

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
			if err := rows.Scan(valuePtrs...); err != nil {
				slog.Warn("Failed to scan row during dump", "tableName", tableName, "error", err)
				continue
			}

			var vals []string
			for _, v := range values {
				if v == nil {
					vals = append(vals, "NULL")
				} else if b, ok := v.([]byte); ok {
					vals = append(vals, fmt.Sprintf("'%s'", s.escapeSQLString(string(b))))
				} else {
					vals = append(vals, fmt.Sprintf("'%v'", v))
				}
			}
			sqlDump.WriteString(fmt.Sprintf("INSERT INTO `%s` VALUES (%s);\n", tableName, strings.Join(vals, ", ")))
		}
		rows.Close()
		sqlDump.WriteString("\n")
	}

	return sqlDump.String(), nil
}

// ResetProjectDatabase drops all system tables in a project database
func (s *DatabaseService) ResetProjectDatabase(dbName, password string) (int, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return 0, err
	}

	rows, _ := db.Query("SHOW TABLES")
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		rows.Scan(&table)
		tables = append(tables, table)
	}

	if _, err := db.Exec("SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		slog.Warn("Failed to disable foreign key checks", "error", err)
	}
	for _, table := range tables {
		if _, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table)); err != nil {
			slog.Warn("Failed to drop table during reset", "table", table, "error", err)
		}
	}
	if _, err := db.Exec("SET FOREIGN_KEY_CHECKS = 1"); err != nil {
		slog.Warn("Failed to enable foreign key checks", "error", err)
	}

	return len(tables), nil
}

// Internal Helpers

func (s *DatabaseService) isValidIdentifier(name string) bool {
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return len(name) > 0 && len(name) < 64
}

func (s *DatabaseService) escapeSQLString(val string) string {
	val = strings.ReplaceAll(val, "\\", "\\\\")
	val = strings.ReplaceAll(val, "'", "\\'")
	val = strings.ReplaceAll(val, "\n", "\\n")
	val = strings.ReplaceAll(val, "\r", "\\r")
	return val
}

// AdminListAllDatabases returns a summary of all student databases
func (s *DatabaseService) AdminListAllDatabases() ([]AdminDatabaseInfo, error) {
	var projects []struct {
		ID               uint
		Name             string
		UserName         string
		DatabaseName     string
		DatabasePassword string
		Status           string
	}

	err := s.db.Table("projects").
		Select("projects.id, projects.name, users.name as user_name, projects.database_name, projects.database_password, projects.status").
		Joins("left join users on users.id = projects.user_id").
		Where("projects.deleted_at IS NULL").
		Scan(&projects).Error

	if err != nil {
		return nil, err
	}

	var result []AdminDatabaseInfo
	for _, p := range projects {
		info := AdminDatabaseInfo{
			ProjectID:    p.ID,
			ProjectName:  p.Name,
			StudentName:  p.UserName,
			DatabaseName: p.DatabaseName,
			Status:       p.Status,
			TableCount:   0,
			Size:         "0 KB",
		}

		// Try to get quick stats from information_schema
		db, err := s.ConnectToProjectDB(p.DatabaseName, p.DatabasePassword)
		if err == nil {
			var tableCount int
			var totalSize float64
			err := db.QueryRow(`
				SELECT 
					COUNT(*),
					COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH) / 1024, 0)
				FROM information_schema.TABLES 
				WHERE TABLE_SCHEMA = ?
			`, p.DatabaseName).Scan(&tableCount, &totalSize)

			if err == nil {
				info.TableCount = tableCount
				info.Size = fmt.Sprintf("%.2f KB", totalSize)
				if totalSize > 1024 {
					info.Size = fmt.Sprintf("%.2f MB", totalSize/1024)
				}
			}
		}

		result = append(result, info)
	}

	return result, nil
}
