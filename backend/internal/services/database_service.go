// ===========================================
// Database Service
// ===========================================
// Orchestrates project database operations
// ===========================================
package services

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"gorm.io/gorm"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type DatabaseService struct {
	db   *gorm.DB
	cfg  *config.Config
	pool sync.Map   // map[string]*sql.DB — cached connections per user database: "engine:dbName"
	mu   sync.Mutex // Protects pool connection initialization against race conditions
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
	Comment  string  `json:"comment"`
}

type ForeignKeyInfo struct {
	ColumnName     string `json:"column_name"`
	TargetTable    string `json:"target_table"`
	TargetColumn   string `json:"target_column"`
	ConstraintName string `json:"constraint_name"`
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
	UserName     string `json:"user_name"`
	DatabaseName string `json:"database_name"`
	Engine       string `json:"engine"`
	TableCount   int    `json:"table_count"`
	Size         string `json:"size"`
	Status       string `json:"status"`
}

type DesignerActionRequest struct {
	Action    string          `json:"action"` // "create_table", "rename_table", "drop_table", "add_column", "drop_column", "create_index"
	TableName string          `json:"table_name"`
	NewName   string          `json:"new_name,omitempty"` // For renames
	Column    *DesignerColumn `json:"column,omitempty"`   // For column add/modify
	IndexName string          `json:"index_name,omitempty"`
	IndexCols []string        `json:"index_columns,omitempty"`
}

type DesignerColumn struct {
	Name         string  `json:"name"`
	Type         string  `json:"type"` // "varchar", "integer", "text", "timestamp", etc.
	Length       int     `json:"length,omitempty"`
	Nullable     bool    `json:"nullable"`
	PrimaryKey   bool    `json:"primary_key"`
	DefaultValue *string `json:"default_value"`
	Unique       bool    `json:"unique"`
	Comment      *string `json:"comment"`
	ForeignKey   bool    `json:"foreign_key"`
	FkTable      string  `json:"fk_table"`
	FkColumn     string  `json:"fk_column"`
	FkOnDelete   string  `json:"fk_on_delete"`
}

// LogAudit persists designer audit records to database
func (s *DatabaseService) LogAudit(log *models.AuditLog) error {
	return s.db.Create(log).Error
}

// getEngineForDB checks the database engine from the DatabaseInstance model
func (s *DatabaseService) getEngineForDB(dbName string) string {
	var instance models.DatabaseInstance
	if err := s.db.Where("name = ?", dbName).First(&instance).Error; err == nil {
		return instance.Engine
	}
	return "mysql" // Fallback to legacy default
}

// ConnectToProjectDB returns a pooled connection to a user's database.
// Uses double-checked locking to guarantee thread safety during connection initialization.
func (s *DatabaseService) ConnectToProjectDB(dbName, password string) (*sql.DB, error) {
	engine := s.getEngineForDB(dbName)
	cacheKey := fmt.Sprintf("%s:%s", engine, dbName)

	// Return cached connection if available and healthy (fast path)
	if cached, ok := s.pool.Load(cacheKey); ok {
		db := cached.(*sql.DB)
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Stale connection: remove and recreate
		s.pool.Delete(cacheKey)
		db.Close()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check cached connection under mutex (slow path)
	if cached, ok := s.pool.Load(cacheKey); ok {
		db := cached.(*sql.DB)
		if err := db.Ping(); err == nil {
			return db, nil
		}
		s.pool.Delete(cacheKey)
		db.Close()
	}

	var db *sql.DB
	var err error

	if engine == "postgresql" {
		// Generate salted application name to prevent client-side spoofing
		appHash := sha256.Sum256([]byte(s.cfg.UIDSalt))
		appName := fmt.Sprintf("paas-backend-%x", appHash[:8])

		dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&application_name=%s",
			dbName, password, s.cfg.UserPGHost, s.cfg.UserPGPort, dbName, appName,
		)
		db, err = sql.Open("pgx", dsn)
	} else {
		// Default to mysql
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
			dbName, password, s.cfg.MYSQLHost, s.cfg.MYSQLPort, dbName,
		)
		db, err = sql.Open("mysql", dsn)
	}

	if err != nil {
		return nil, err
	}

	// Conservative pool limits per user database
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	s.pool.Store(cacheKey, db)
	return db, nil
}

// GetDatabaseInstanceByProjectID loads the DatabaseInstance record for a project
func (s *DatabaseService) GetDatabaseInstanceByProjectID(projectID uint) (*models.DatabaseInstance, error) {
	var instance models.DatabaseInstance
	if err := s.db.Where("project_id = ?", projectID).First(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

// ListProjectTables returns metadata for all tables in a project database
func (s *DatabaseService) ListProjectTables(dbName, password string) ([]TableInfo, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var rows *sql.Rows
	if engine == "postgresql" {
		rows, err = db.QueryContext(ctx, `
			SELECT 
				t.tablename,
				COALESCE(c.reltuples::bigint, -1) AS table_rows,
				ROUND((pg_total_relation_size(quote_ident(t.tablename)) / 1024.0), 2) AS size_kb,
				'PostgreSQL' AS engine,
				NOW() AS create_time
			FROM pg_tables t
			LEFT JOIN pg_class c ON c.relname = t.tablename
			WHERE t.schemaname = 'public'
			ORDER BY t.tablename
		`)
	} else {
		rows, err = db.QueryContext(ctx, `
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
	}

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

		if engine == "postgresql" && t.Rows < 0 {
			// Table is unanalyzed/new (pg_class.reltuples returned -1).
			// Fetch the exact, real-time count using a quick COUNT(*).
			escapedTableName := s.escapeIdentifier(engine, t.Name)
			var realCount int64
			if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", escapedTableName)).Scan(&realCount); err == nil {
				t.Rows = realCount
			} else {
				t.Rows = 0
			}
		}

		t.Size = fmt.Sprintf("%.2f KB", sizeKB)
		if created.Valid {
			t.Created = created.Time.Format("2006-01-02 15:04")
		} else {
			t.Created = time.Now().Format("2006-01-02 15:04")
		}
		tables = append(tables, t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tables, nil
}

// GetTableStructure returns column metadata for a specific table
func (s *DatabaseService) GetTableStructure(dbName, password, tableName string) ([]ColumnInfo, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var rows *sql.Rows
	if engine == "postgresql" {
		rows, err = db.QueryContext(ctx, `
			SELECT 
				c.column_name,
				c.data_type,
				c.is_nullable,
				COALESCE(
					(SELECT 'PRI' FROM pg_index i 
					 JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
					 WHERE i.indrelid = quote_ident(c.table_name)::regclass AND i.indisprimary AND a.attname = c.column_name LIMIT 1), 
					''
				) AS column_key,
				c.column_default,
				'' AS extra,
				COALESCE(pg_catalog.col_description(quote_ident(c.table_name)::regclass, c.ordinal_position), '') AS column_comment
			FROM information_schema.columns c
			WHERE c.table_schema = 'public' AND c.table_name = $1
			ORDER BY c.ordinal_position
		`, tableName)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT 
				COLUMN_NAME,
				COLUMN_TYPE,
				IS_NULLABLE,
				COLUMN_KEY,
				COLUMN_DEFAULT,
				EXTRA,
				COLUMN_COMMENT
			FROM information_schema.COLUMNS 
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
			ORDER BY ORDINAL_POSITION
		`, dbName, tableName)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var nullable string
		if err := rows.Scan(&col.Name, &col.Type, &nullable, &col.Key, &col.Default, &col.Extra, &col.Comment); err != nil {
			continue
		}
		col.Nullable = (nullable == "YES" || nullable == "yes" || nullable == "TRUE" || nullable == "true")
		columns = append(columns, col)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return columns, nil
}

func (s *DatabaseService) GetTableForeignKeys(dbName, password, tableName string) ([]ForeignKeyInfo, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var rows *sql.Rows
	if engine == "postgresql" {
		rows, err = db.QueryContext(ctx, `
			SELECT
				kcu.column_name,
				ccu.table_name AS referenced_table_name,
				ccu.column_name AS referenced_column_name,
				tc.constraint_name
			FROM
				information_schema.table_constraints AS tc
				JOIN information_schema.key_column_usage AS kcu
				  ON tc.constraint_name = kcu.constraint_name
				  AND tc.table_schema = kcu.table_schema
				JOIN information_schema.constraint_column_usage AS ccu
				  ON ccu.constraint_name = tc.constraint_name
				  AND ccu.table_schema = tc.table_schema
			WHERE 
				tc.constraint_type = 'FOREIGN KEY' 
				AND tc.table_schema = 'public' 
				AND tc.table_name = $1;
		`, tableName)
	} else {
		rows, err = db.QueryContext(ctx, `
			SELECT 
				COLUMN_NAME,
				REFERENCED_TABLE_NAME,
				REFERENCED_COLUMN_NAME,
				CONSTRAINT_NAME
			FROM 
				INFORMATION_SCHEMA.KEY_COLUMN_USAGE
			WHERE 
				TABLE_SCHEMA = ? 
				AND TABLE_NAME = ? 
				AND REFERENCED_TABLE_NAME IS NOT NULL;
		`, dbName, tableName)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fks := []ForeignKeyInfo{}
	for rows.Next() {
		var fk ForeignKeyInfo
		if err := rows.Scan(&fk.ColumnName, &fk.TargetTable, &fk.TargetColumn, &fk.ConstraintName); err != nil {
			continue
		}
		fks = append(fks, fk)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return fks, nil
}

// GetTableData supports paginated data retrieval from a table
func (s *DatabaseService) GetTableData(dbName, password, tableName string, page, limit int) ([]string, []map[string]interface{}, int64, error) {
	if !s.isValidIdentifier(tableName) {
		return nil, nil, 0, apperr.New(400, "INVALID_TABLE_NAME", "Table name contains invalid characters")
	}

	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, nil, 0, err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	escapedTableName := s.escapeIdentifier(engine, tableName)

	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", escapedTableName)
	if err := db.QueryRowContext(ctx, countQuery).Scan(&total); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to count rows: %w", err)
	}

	if limit > 200 {
		limit = 200
	}

	offset := (page - 1) * limit
	query := fmt.Sprintf("SELECT * FROM %s LIMIT %d OFFSET %d", escapedTableName, limit, offset)
	rows, err := db.QueryContext(ctx, query)
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

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	escapedTable := s.escapeIdentifier(engine, tableName)
	escapedCol := s.escapeIdentifier(engine, pkColumn)

	var query string
	if engine == "postgresql" {
		query = fmt.Sprintf("DELETE FROM %s WHERE %s = $1", escapedTable, escapedCol)
	} else {
		query = fmt.Sprintf("DELETE FROM %s WHERE %s = ? LIMIT 1", escapedTable, escapedCol)
	}

	result, err := db.ExecContext(ctx, query, pkValue)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

// UpdateTableRow safely updates a specific row's fields in a table using a primary key
func (s *DatabaseService) UpdateTableRow(dbName, password, tableName, pkColumn string, pkValue interface{}, updates map[string]interface{}) (int64, error) {
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

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	escapedTable := s.escapeIdentifier(engine, tableName)
	escapedPkCol := s.escapeIdentifier(engine, pkColumn)

	var setClauses []string
	var args []interface{}
	placeholderIdx := 1

	for col, val := range updates {
		if !s.isValidIdentifier(col) {
			return 0, apperr.New(400, "INVALID_COLUMN_NAME", "Column name contains invalid characters")
		}
		escapedCol := s.escapeIdentifier(engine, col)
		if engine == "postgresql" {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", escapedCol, placeholderIdx))
		} else {
			setClauses = append(setClauses, fmt.Sprintf("%s = ?", escapedCol))
		}
		args = append(args, val)
		placeholderIdx++
	}

	if len(setClauses) == 0 {
		return 0, apperr.New(400, "NO_UPDATES", "No updates provided")
	}

	var query string
	if engine == "postgresql" {
		query = fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d", escapedTable, strings.Join(setClauses, ", "), escapedPkCol, placeholderIdx)
	} else {
		query = fmt.Sprintf("UPDATE %s SET %s WHERE %s = ? LIMIT 1", escapedTable, strings.Join(setClauses, ", "), escapedPkCol)
	}
	args = append(args, pkValue)

	result, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

// ExecuteRawQuery runs a manual SQL query against a project database under a 15-second execution timeout.
func (s *DatabaseService) ExecuteRawQuery(dbName, password, query string) (*QueryResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, apperr.New(400, "EMPTY_QUERY", "Query cannot be empty")
	}

	// Reject multi-statement queries to prevent PostgreSQL semicolon bypasses.
	statements := SplitSQLStatements(query)
	if len(statements) > 1 {
		return nil, apperr.New(403, "SQL_MULTIPLE_STATEMENTS", "Executing multiple SQL statements in a single query is not permitted")
	} else if len(statements) == 0 {
		return nil, apperr.New(400, "EMPTY_QUERY", "Query cannot be empty")
	}

	// Enforce the single parsed statement.
	query = statements[0]
	upperQuery := strings.ToUpper(query)
	normalizedQuery := normalizeSQLForCheck(query)

	// Block dangerous operations (normalized in UPPERCASE)
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
		"XP_",
		"INFORMATION_SCHEMA.PROCESSLIST",
		"MYSQL.",
		"PERFORMANCE_SCHEMA.",
		"SYS.",
		"UNION SELECT",
		"SELECT.*INTO",
	}

	for _, pattern := range blockedPatterns {
		if strings.Contains(pattern, ".*") {
			re, err := regexp.Compile(pattern)
			if err == nil && re.MatchString(normalizedQuery) {
				slog.Warn("Blocked forbidden SQL operation attempt",
					"database", dbName,
					"pattern", pattern,
				)
				return nil, apperr.New(403, "SQL_OPERATION_FORBIDDEN", "This SQL operation is not permitted for security reasons")
			}
		} else {
			if strings.Contains(normalizedQuery, pattern) {
				slog.Warn("Blocked forbidden SQL operation attempt",
					"database", dbName,
					"pattern", pattern,
				)
				return nil, apperr.New(403, "SQL_OPERATION_FORBIDDEN", "This SQL operation is not permitted for security reasons")
			}
		}
	}

	// Allow core commands
	allowedPrefixes := []string{"SELECT", "SHOW", "DESCRIBE", "DESC", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP"}
	allowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(normalizedQuery, prefix) {
			allowed = true
			break
		}
	}

	if !allowed {
		return nil, apperr.New(403, "SQL_OPERATION_FORBIDDEN", "Only SELECT, INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, SHOW, and DESCRIBE are allowed")
	}

	// Block cross-database queries
	if strings.Contains(query, ".") && !strings.Contains(normalizedQuery, "INFORMATION_SCHEMA") && !strings.Contains(normalizedQuery, "PG_") {
		parts := strings.Split(query, ".")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, "`\"")
			partUpper := strings.ToUpper(part)
			dbNameUpper := strings.ToUpper(dbName)
			if partUpper != dbNameUpper && partUpper != "INFORMATION_SCHEMA" && partUpper != "PUBLIC" && len(part) > 0 && !strings.HasPrefix(partUpper, "SELECT") {
				if strings.Contains(normalizedQuery, "`"+partUpper+"`") || strings.Contains(normalizedQuery, "\""+partUpper+"\"") || strings.Contains(normalizedQuery, partUpper+".") {
					slog.Warn("Blocked cross-database query attempt",
						"database", dbName,
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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()

	// Handle data retrieval queries
	if strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "SHOW") || strings.HasPrefix(upperQuery, "DESCRIBE") || strings.HasPrefix(upperQuery, "DESC") {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			slog.Warn("SQL query execution failed", "database", dbName, "error", err.Error())
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
	result, err := db.ExecContext(ctx, query)
	if err != nil {
		slog.Warn("SQL modification query failed", "database", dbName, "error", err.Error())
		return nil, apperr.New(400, "QUERY_ERROR", "Query execution failed")
	}

	affected, _ := result.RowsAffected()
	return &QueryResult{
		RowsAffected: affected,
		Duration:     time.Since(start).String(),
	}, nil
}

// GenerateProjectDump creates a logical SQL export for a project database.
// Enforces a strict 100MB database size cap to prevent OOM / SRE resource exhaustion crashes.
func (s *DatabaseService) GenerateProjectDump(dbName, password string) (string, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return "", err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Enforce SRE safety cap: check database size to prevent memory exhaustion (OOM)
	var totalSize float64
	if engine == "postgresql" {
		_ = db.QueryRowContext(ctx, "SELECT COALESCE(SUM(pg_total_relation_size(quote_ident(tablename)) / 1024.0), 0) FROM pg_tables WHERE schemaname = 'public'").Scan(&totalSize)
	} else {
		_ = db.QueryRowContext(ctx, `
			SELECT COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH) / 1024.0, 0)
			FROM information_schema.TABLES 
			WHERE TABLE_SCHEMA = ?
		`, dbName).Scan(&totalSize)
	}

	if totalSize > 102400 { // 100MB in KB
		return "", apperr.New(413, "DATABASE_TOO_LARGE", fmt.Sprintf("Database size (%.2f MB) exceeds the 100MB safety ceiling for dynamic logical exports. Please utilize SRE CLI utilities.", totalSize/1024.0))
	}

	var sqlDump strings.Builder
	sqlDump.WriteString(fmt.Sprintf("-- Database Export: %s\n", dbName))
	sqlDump.WriteString(fmt.Sprintf("-- Engine: %s\n", engine))
	sqlDump.WriteString(fmt.Sprintf("-- Generated: %s\n\n", time.Now().Format(time.RFC3339)))

	var tables []string
	if engine == "postgresql" {
		rows, err := db.QueryContext(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
		if err != nil {
			return "", err
		}
		defer rows.Close()
		for rows.Next() {
			var tbl string
			if err := rows.Scan(&tbl); err == nil {
				tables = append(tables, tbl)
			}
		}
	} else {
		rows, err := db.QueryContext(ctx, "SHOW TABLES")
		if err != nil {
			return "", err
		}
		defer rows.Close()
		for rows.Next() {
			var tbl string
			if err := rows.Scan(&tbl); err == nil {
				tables = append(tables, tbl)
			}
		}
	}

	for _, tableName := range tables {
		escapedTable := s.escapeIdentifier(engine, tableName)

		if engine == "postgresql" {
			// Construct mock CREATE TABLE for PostgreSQL using information_schema columns
			sqlDump.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))
			sqlDump.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;\n", escapedTable))
			sqlDump.WriteString(fmt.Sprintf("CREATE TABLE %s (\n", escapedTable))

			cols, err := db.QueryContext(ctx, `
				SELECT column_name, data_type, is_nullable, column_default 
				FROM information_schema.columns 
				WHERE table_schema = 'public' AND table_name = $1
				ORDER BY ordinal_position
			`, tableName)
			if err == nil {
				var colDefs []string
				for cols.Next() {
					var colName, dataType, isNullable string
					var colDefault sql.NullString
					if err := cols.Scan(&colName, &dataType, &isNullable, &colDefault); err == nil {
						def := fmt.Sprintf("  %s %s", s.escapeIdentifier(engine, colName), dataType)
						if isNullable == "NO" {
							def += " NOT NULL"
						}
						if colDefault.Valid {
							def += " DEFAULT " + colDefault.String
						}
						colDefs = append(colDefs, def)
					}
				}
				cols.Close()
				sqlDump.WriteString(strings.Join(colDefs, ",\n"))
			}
			sqlDump.WriteString("\n);\n\n")
		} else {
			var tblName, createStmt string
			err := db.QueryRowContext(ctx, fmt.Sprintf("SHOW CREATE TABLE %s", escapedTable)).Scan(&tblName, &createStmt)
			if err != nil {
				continue
			}
			sqlDump.WriteString(fmt.Sprintf("-- Table: %s\n", tableName))
			sqlDump.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS %s;\n", escapedTable))
			sqlDump.WriteString(createStmt + ";\n\n")
		}

		// Insert rows
		rows, err := db.QueryContext(ctx, fmt.Sprintf("SELECT * FROM %s", escapedTable))
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
			sqlDump.WriteString(fmt.Sprintf("INSERT INTO %s VALUES (%s);\n", escapedTable, strings.Join(vals, ", ")))
		}
		rows.Close()
		sqlDump.WriteString("\n")
	}

	return sqlDump.String(), nil
}

// ResetProjectDatabase drops all tables in a database
func (s *DatabaseService) ResetProjectDatabase(dbName, password string) (int, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return 0, err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var tables []string
	if engine == "postgresql" {
		rows, _ := db.QueryContext(ctx, "SELECT tablename FROM pg_tables WHERE schemaname = 'public'")
		defer rows.Close()
		for rows.Next() {
			var tbl string
			if err := rows.Scan(&tbl); err == nil {
				tables = append(tables, tbl)
			}
		}
		for _, tbl := range tables {
			_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", s.escapeIdentifier(engine, tbl)))
		}
	} else {
		rows, _ := db.QueryContext(ctx, "SHOW TABLES")
		defer rows.Close()
		for rows.Next() {
			var tbl string
			if err := rows.Scan(&tbl); err == nil {
				tables = append(tables, tbl)
			}
		}
		_, _ = db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0")
		for _, tbl := range tables {
			_, _ = db.ExecContext(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", s.escapeIdentifier(engine, tbl)))
		}
		_, _ = db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")
	}

	return len(tables), nil
}

// ExecuteDesignerAction generates and executes visual DDL transformations on MySQL or PostgreSQL.
func (s *DatabaseService) ExecuteDesignerAction(dbName, password string, req DesignerActionRequest) error {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return err
	}

	engine := s.getEngineForDB(dbName)
	// DDL operations may wait for Metadata Locks if the application has active transactions.
	// We use a longer timeout (45s) to allow locks to release gracefully without timing out the UI prematurely.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if !s.isValidIdentifier(req.TableName) {
		return apperr.New(400, "INVALID_TABLE_NAME", "Table name is invalid")
	}

	escapedTable := s.escapeIdentifier(engine, req.TableName)
	var sqlQuery string

	// Acquire a dedicated connection from the pool to enforce session-level variables
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Initialize transactional execution boundaries.
	var tx *sql.Tx
	var execer interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	} = conn

	if engine == "postgresql" {
		// Enforce a strict 2-second lock wait timeout to prevent DDL from causing a metadata lock DoS
		_, _ = conn.ExecContext(ctx, "SET lock_timeout = '2s';")
		var txErr error
		tx, txErr = conn.BeginTx(ctx, nil)
		if txErr != nil {
			return txErr
		}
		defer func() {
			_ = tx.Rollback()
		}()
		execer = tx
	} else {
		// Enforce a strict 2-second lock wait timeout to prevent DDL from causing a metadata lock DoS
		_, _ = conn.ExecContext(ctx, "SET SESSION lock_wait_timeout = 2;")
	}

	switch req.Action {
	case "create_table":
		if req.Column == nil {
			return apperr.New(400, "COLUMN_REQUIRED", "At least one primary key column is required to create a table")
		}
		if !s.isValidIdentifier(req.Column.Name) {
			return apperr.New(400, "INVALID_COLUMN_NAME", "Column name is invalid")
		}
		escapedCol := s.escapeIdentifier(engine, req.Column.Name)

		if engine == "postgresql" {
			colDef := fmt.Sprintf("%s SERIAL PRIMARY KEY", escapedCol)
			sqlQuery = fmt.Sprintf("CREATE TABLE %s (%s);", escapedTable, colDef)
		} else {
			colDef := fmt.Sprintf("%s INT AUTO_INCREMENT PRIMARY KEY", escapedCol)
			sqlQuery = fmt.Sprintf("CREATE TABLE %s (%s) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;", escapedTable, colDef)
		}

	case "rename_table":
		if !s.isValidIdentifier(req.NewName) {
			return apperr.New(400, "INVALID_NEW_NAME", "New table name is invalid")
		}
		escapedNewTable := s.escapeIdentifier(engine, req.NewName)
		if engine == "postgresql" {
			sqlQuery = fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", escapedTable, s.escapeIdentifier(engine, req.NewName))
		} else {
			sqlQuery = fmt.Sprintf("RENAME TABLE %s TO %s;", escapedTable, escapedNewTable)
		}

	case "drop_table":
		if engine == "postgresql" {
			sqlQuery = fmt.Sprintf("DROP TABLE %s CASCADE;", escapedTable)
		} else {
			sqlQuery = fmt.Sprintf("DROP TABLE %s;", escapedTable)
		}

	case "drop_column":
		if req.IndexName == "" || !s.isValidIdentifier(req.IndexName) {
			return apperr.New(400, "COLUMN_REQUIRED", "Valid column name is required to drop a column")
		}
		escapedCol := s.escapeIdentifier(engine, req.IndexName)
		if engine == "postgresql" {
			sqlQuery = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s CASCADE;", escapedTable, escapedCol)
		} else {
			sqlQuery = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", escapedTable, escapedCol)
		}

	case "create_index":
		if req.IndexName == "" || !s.isValidIdentifier(req.IndexName) {
			return apperr.New(400, "INDEX_NAME_REQUIRED", "Valid index name is required")
		}
		if len(req.IndexCols) == 0 {
			return apperr.New(400, "COLUMNS_REQUIRED", "At least one column is required to create an index")
		}
		var escapedCols []string
		for _, col := range req.IndexCols {
			if !s.isValidIdentifier(col) {
				return apperr.New(400, "INVALID_COLUMN_NAME", "Column name in index is invalid")
			}
			escapedCols = append(escapedCols, s.escapeIdentifier(engine, col))
		}
		escapedIndex := s.escapeIdentifier(engine, req.IndexName)
		colsList := strings.Join(escapedCols, ", ")
		sqlQuery = fmt.Sprintf("CREATE INDEX %s ON %s (%s);", escapedIndex, escapedTable, colsList)

	case "add_column":
		if req.Column == nil || !s.isValidIdentifier(req.Column.Name) {
			return apperr.New(400, "COLUMN_REQUIRED", "Valid column name is required")
		}
		escapedCol := s.escapeIdentifier(engine, req.Column.Name)
		dbType := s.mapDesignerType(engine, req.Column.Type, req.Column.Length)

		nullability := "NULL"
		if !req.Column.Nullable {
			nullability = "NOT NULL"
		}

		defaultClause := ""
		if req.Column.DefaultValue != nil {
			if strings.ToUpper(strings.TrimSpace(*req.Column.DefaultValue)) == "NULL" {
				defaultClause = " DEFAULT NULL"
			} else {
				defaultClause = fmt.Sprintf(" DEFAULT '%s'", s.escapeSQLString(*req.Column.DefaultValue))
			}
		}

		commentClause := ""
		if engine == "mysql" && req.Column.Comment != nil && *req.Column.Comment != "" {
			commentClause = fmt.Sprintf(" COMMENT '%s'", s.escapeSQLString(*req.Column.Comment))
		}

		sqlQuery = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s %s%s%s;",
			escapedTable, escapedCol, dbType, nullability, defaultClause, commentClause,
		)
		if _, err = execer.ExecContext(ctx, sqlQuery); err != nil {
			return err
		}

		// Comment for PostgreSQL
		if engine == "postgresql" && req.Column.Comment != nil && *req.Column.Comment != "" {
			commentSql := fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s';", escapedTable, escapedCol, s.escapeSQLString(*req.Column.Comment))
			if _, err = execer.ExecContext(ctx, commentSql); err != nil {
				return err
			}
		}

		// Primary Key Constraint
		if req.Column.PrimaryKey {
			var pkSql string
			if engine == "postgresql" {
				pkeyName := s.escapeIdentifier(engine, req.TableName+"_pkey")
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", escapedTable, pkeyName))
				pkSql = fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", escapedTable, escapedCol)
			} else {
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY;", escapedTable))
				pkSql = fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", escapedTable, escapedCol)
			}
			if _, err = execer.ExecContext(ctx, pkSql); err != nil {
				return err
			}
		}

		// Unique Constraint
		if req.Column.Unique {
			uqName := fmt.Sprintf("uq_%s_%s", req.TableName, req.Column.Name)
			escapedUq := s.escapeIdentifier(engine, uqName)
			var uqSql string
			if engine == "postgresql" {
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", escapedTable, escapedUq))
				uqSql = fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", escapedTable, escapedUq, escapedCol)
			} else {
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP INDEX %s;", escapedTable, escapedUq))
				uqSql = fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", escapedTable, escapedUq, escapedCol)
			}
			if _, err = execer.ExecContext(ctx, uqSql); err != nil {
				return err
			}
		}

		// Foreign Key Constraint
		if req.Column.ForeignKey && req.Column.FkTable != "" && req.Column.FkColumn != "" {
			if !s.isValidIdentifier(req.Column.FkTable) || !s.isValidIdentifier(req.Column.FkColumn) {
				return apperr.New(400, "INVALID_FK_IDENTIFIER", "Foreign key target table or column is invalid")
			}
			escapedTargetTable := s.escapeIdentifier(engine, req.Column.FkTable)
			escapedTargetCol := s.escapeIdentifier(engine, req.Column.FkColumn)
			fkName := fmt.Sprintf("fk_%s_%s", req.TableName, req.Column.Name)
			escapedFk := s.escapeIdentifier(engine, fkName)

			onDelete := "NO ACTION"
			switch strings.ToUpper(req.Column.FkOnDelete) {
			case "CASCADE":
				onDelete = "CASCADE"
			case "SET NULL":
				onDelete = "SET NULL"
			case "RESTRICT":
				onDelete = "RESTRICT"
			case "NO ACTION":
				onDelete = "NO ACTION"
			}

			var fkSql string
			if engine == "postgresql" {
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", escapedTable, escapedFk))
				fkSql = fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE %s;",
					escapedTable, escapedFk, escapedCol, escapedTargetTable, escapedTargetCol, onDelete)
			} else {
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s;", escapedTable, escapedFk))
				fkSql = fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE %s;",
					escapedTable, escapedFk, escapedCol, escapedTargetTable, escapedTargetCol, onDelete)
			}
			if _, err = execer.ExecContext(ctx, fkSql); err != nil {
				return err
			}
		}

		if tx != nil {
			if err = tx.Commit(); err != nil {
				return err
			}
		}
		return nil

	case "modify_column":
		if req.Column == nil || !s.isValidIdentifier(req.Column.Name) {
			return apperr.New(400, "COLUMN_REQUIRED", "Valid column name is required")
		}

		// Retrieve current schema constraints to avoid needless dropping/creation races
		var originalCol *ColumnInfo
		currentCols, _ := s.GetTableStructure(dbName, password, req.TableName)
		for _, c := range currentCols {
			if c.Name == req.Column.Name {
				originalCol = &c
				break
			}
		}

		var originalFk *ForeignKeyInfo
		currentFks, _ := s.GetTableForeignKeys(dbName, password, req.TableName)
		for _, f := range currentFks {
			if f.ColumnName == req.Column.Name {
				originalFk = &f
				break
			}
		}

		escapedCol := s.escapeIdentifier(engine, req.Column.Name)
		dbType := s.mapDesignerType(engine, req.Column.Type, req.Column.Length)

		nullability := "NULL"
		if !req.Column.Nullable {
			nullability = "NOT NULL"
		}

		defaultClause := ""
		if req.Column.DefaultValue != nil {
			if strings.ToUpper(strings.TrimSpace(*req.Column.DefaultValue)) == "NULL" {
				defaultClause = " DEFAULT NULL"
			} else {
				defaultClause = fmt.Sprintf(" DEFAULT '%s'", s.escapeSQLString(*req.Column.DefaultValue))
			}
		}

		if engine == "postgresql" {
			// 1. Alter Type (with USING cast)
			typeCast := fmt.Sprintf("TYPE %s USING %s::%s", dbType, escapedCol, dbType)
			sqlQuery1 := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s;", escapedTable, escapedCol, typeCast)
			if _, err = execer.ExecContext(ctx, sqlQuery1); err != nil {
				return err
			}

			// 2. Alter Nullability
			nullAction := "DROP NOT NULL"
			if !req.Column.Nullable {
				nullAction = "SET NOT NULL"
			}
			sqlQuery2 := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s;", escapedTable, escapedCol, nullAction)
			if _, err = execer.ExecContext(ctx, sqlQuery2); err != nil {
				return err
			}

			// 3. Alter Default
			defaultAction := "DROP DEFAULT"
			if req.Column.DefaultValue != nil {
				if strings.ToUpper(strings.TrimSpace(*req.Column.DefaultValue)) == "NULL" {
					defaultAction = "SET DEFAULT NULL"
				} else {
					defaultAction = fmt.Sprintf("SET DEFAULT '%s'", s.escapeSQLString(*req.Column.DefaultValue))
				}
			}
			sqlQuery3 := fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s;", escapedTable, escapedCol, defaultAction)
			if _, err = execer.ExecContext(ctx, sqlQuery3); err != nil {
				return err
			}

			// 4. Conditional Foreign Key Drop
			wasFk := originalFk != nil
			isFk := req.Column.ForeignKey && req.Column.FkTable != "" && req.Column.FkColumn != ""
			fkChanged := wasFk != isFk || (wasFk && isFk && (originalFk.TargetTable != req.Column.FkTable || originalFk.TargetColumn != req.Column.FkColumn))
			if fkChanged && wasFk {
				oldFkName := s.escapeIdentifier(engine, fmt.Sprintf("fk_%s_%s", req.TableName, req.Column.Name))
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", escapedTable, oldFkName))
			}

			// 5. Conditional Unique Key Drop
			wasUq := originalCol != nil && originalCol.Key == "UNI"
			isUq := req.Column.Unique
			uqChanged := wasUq != isUq
			if uqChanged && wasUq {
				oldUqName := s.escapeIdentifier(engine, fmt.Sprintf("uq_%s_%s", req.TableName, req.Column.Name))
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", escapedTable, oldUqName))
			}

			// Rename column (if NewName is provided and is different)
			colNameAfterRename := req.Column.Name
			if req.NewName != "" && req.NewName != req.Column.Name {
				if !s.isValidIdentifier(req.NewName) {
					return apperr.New(400, "INVALID_NEW_NAME", "New column name is invalid")
				}
				sqlQueryRename := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", escapedTable, escapedCol, s.escapeIdentifier(engine, req.NewName))
				if _, err = execer.ExecContext(ctx, sqlQueryRename); err != nil {
					return err
				}
				colNameAfterRename = req.NewName
			}
			escapedColAfterRename := s.escapeIdentifier(engine, colNameAfterRename)

			// Comment
			if req.Column.Comment != nil {
				var commentSql string
				if *req.Column.Comment == "" {
					commentSql = fmt.Sprintf("COMMENT ON COLUMN %s.%s IS NULL;", escapedTable, escapedColAfterRename)
				} else {
					commentSql = fmt.Sprintf("COMMENT ON COLUMN %s.%s IS '%s';", escapedTable, escapedColAfterRename, s.escapeSQLString(*req.Column.Comment))
				}
				if _, err = execer.ExecContext(ctx, commentSql); err != nil {
					return err
				}
			}

			// Primary Key (if requested and changed)
			wasPk := originalCol != nil && originalCol.Key == "PRI"
			isPk := req.Column.PrimaryKey
			pkChanged := wasPk != isPk
			if pkChanged {
				pkeyName := s.escapeIdentifier(engine, req.TableName+"_pkey")
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", escapedTable, pkeyName))
				if isPk {
					pkSql := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", escapedTable, escapedColAfterRename)
					if _, err = execer.ExecContext(ctx, pkSql); err != nil {
						return err
					}
				}
			}

			// Unique (if requested and changed/renamed)
			if isUq && (uqChanged || colNameAfterRename != req.Column.Name) {
				newUqName := s.escapeIdentifier(engine, fmt.Sprintf("uq_%s_%s", req.TableName, colNameAfterRename))
				uqSql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", escapedTable, newUqName, escapedColAfterRename)
				if _, err = execer.ExecContext(ctx, uqSql); err != nil {
					return err
				}
			}

			// Foreign Key (if requested and changed/renamed)
			if isFk && (fkChanged || colNameAfterRename != req.Column.Name) {
				if !s.isValidIdentifier(req.Column.FkTable) || !s.isValidIdentifier(req.Column.FkColumn) {
					return apperr.New(400, "INVALID_FK_IDENTIFIER", "Foreign key target table or column is invalid")
				}
				escapedTargetTable := s.escapeIdentifier(engine, req.Column.FkTable)
				escapedTargetCol := s.escapeIdentifier(engine, req.Column.FkColumn)
				newFkName := s.escapeIdentifier(engine, fmt.Sprintf("fk_%s_%s", req.TableName, colNameAfterRename))

				onDelete := "NO ACTION"
				switch strings.ToUpper(req.Column.FkOnDelete) {
				case "CASCADE":
					onDelete = "CASCADE"
				case "SET NULL":
					onDelete = "SET NULL"
				case "RESTRICT":
					onDelete = "RESTRICT"
				case "NO ACTION":
					onDelete = "NO ACTION"
				}

				fkSql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE %s;",
					escapedTable, newFkName, escapedColAfterRename, escapedTargetTable, escapedTargetCol, onDelete)
				if _, err = execer.ExecContext(ctx, fkSql); err != nil {
					return err
				}
			}

			if tx != nil {
				if err = tx.Commit(); err != nil {
					return err
				}
			}
			return nil

		} else {
			// MySQL: MODIFY COLUMN
			commentClause := ""
			if req.Column.Comment != nil && *req.Column.Comment != "" {
				commentClause = fmt.Sprintf(" COMMENT '%s'", s.escapeSQLString(*req.Column.Comment))
			}

			sqlQuery = fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s %s%s%s;",
				escapedTable, escapedCol, dbType, nullability, defaultClause, commentClause,
			)
			if _, err = execer.ExecContext(ctx, sqlQuery); err != nil {
				return err
			}

			// Conditional Foreign Key Drop (MySQL)
			wasFk := originalFk != nil
			isFk := req.Column.ForeignKey && req.Column.FkTable != "" && req.Column.FkColumn != ""
			fkChanged := wasFk != isFk || (wasFk && isFk && (originalFk.TargetTable != req.Column.FkTable || originalFk.TargetColumn != req.Column.FkColumn))
			if fkChanged && wasFk {
				oldFkName := s.escapeIdentifier(engine, fmt.Sprintf("fk_%s_%s", req.TableName, req.Column.Name))
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP FOREIGN KEY %s;", escapedTable, oldFkName))
			}

			// Conditional Unique Key Drop (MySQL)
			wasUq := originalCol != nil && originalCol.Key == "UNI"
			isUq := req.Column.Unique
			uqChanged := wasUq != isUq
			if uqChanged && wasUq {
				oldUqName := s.escapeIdentifier(engine, fmt.Sprintf("uq_%s_%s", req.TableName, req.Column.Name))
				_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP INDEX %s;", escapedTable, oldUqName))
			}

			// Rename column (if NewName is provided and is different)
			colNameAfterRename := req.Column.Name
			if req.NewName != "" && req.NewName != req.Column.Name {
				if !s.isValidIdentifier(req.NewName) {
					return apperr.New(400, "INVALID_NEW_NAME", "New column name is invalid")
				}
				sqlQueryRename := fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;", escapedTable, escapedCol, s.escapeIdentifier(engine, req.NewName))
				if _, err = execer.ExecContext(ctx, sqlQueryRename); err != nil {
					return err
				}
				colNameAfterRename = req.NewName
			}
			escapedColAfterRename := s.escapeIdentifier(engine, colNameAfterRename)

			// Primary Key (if requested and changed/renamed)
			wasPk := originalCol != nil && originalCol.Key == "PRI"
			isPk := req.Column.PrimaryKey
			pkChanged := wasPk != isPk
			if pkChanged {
				if wasPk {
					_, _ = execer.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s DROP PRIMARY KEY;", escapedTable))
				}
				if isPk {
					pkSql := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", escapedTable, escapedColAfterRename)
					if _, err = execer.ExecContext(ctx, pkSql); err != nil {
						return err
					}
				}
			}

			// Unique (if requested and changed/renamed)
			if isUq && (uqChanged || colNameAfterRename != req.Column.Name) {
				newUqName := s.escapeIdentifier(engine, fmt.Sprintf("uq_%s_%s", req.TableName, colNameAfterRename))
				uqSql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s UNIQUE (%s);", escapedTable, newUqName, escapedColAfterRename)
				if _, err = execer.ExecContext(ctx, uqSql); err != nil {
					return err
				}
			}

			// Foreign Key (if requested and changed/renamed)
			if isFk && (fkChanged || colNameAfterRename != req.Column.Name) {
				if !s.isValidIdentifier(req.Column.FkTable) || !s.isValidIdentifier(req.Column.FkColumn) {
					return apperr.New(400, "INVALID_FK_IDENTIFIER", "Foreign key target table or column is invalid")
				}
				escapedTargetTable := s.escapeIdentifier(engine, req.Column.FkTable)
				escapedTargetCol := s.escapeIdentifier(engine, req.Column.FkColumn)
				newFkName := s.escapeIdentifier(engine, fmt.Sprintf("fk_%s_%s", req.TableName, colNameAfterRename))

				onDelete := "NO ACTION"
				switch strings.ToUpper(req.Column.FkOnDelete) {
				case "CASCADE":
					onDelete = "CASCADE"
				case "SET NULL":
					onDelete = "SET NULL"
				case "RESTRICT":
					onDelete = "RESTRICT"
				case "NO ACTION":
					onDelete = "NO ACTION"
				}

				fkSql := fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s) ON DELETE %s;",
					escapedTable, newFkName, escapedColAfterRename, escapedTargetTable, escapedTargetCol, onDelete)
				if _, err = execer.ExecContext(ctx, fkSql); err != nil {
					return err
				}
			}

			if tx != nil {
				if err = tx.Commit(); err != nil {
					return err
				}
			}
			return nil
		}

	default:
		return apperr.New(400, "UNKNOWN_ACTION", "Visual designer action is unrecognized")
	}

	_, err = execer.ExecContext(ctx, sqlQuery)
	if err == nil && tx != nil {
		err = tx.Commit()
	}
	return err
}

// mapDesignerType maps simple frontend types to SQL engine datatypes.
func (s *DatabaseService) mapDesignerType(engine, dtype string, length int) string {
	dtype = strings.ToLower(dtype)
	switch dtype {
	case "varchar", "string":
		if length <= 0 {
			length = 255
		}
		return fmt.Sprintf("VARCHAR(%d)", length)
	case "char":
		if length <= 0 {
			length = 1
		}
		return fmt.Sprintf("CHAR(%d)", length)
	case "integer", "int":
		return "INT"
	case "bigint":
		return "BIGINT"
	case "decimal":
		return "DECIMAL(10,2)"
	case "double":
		if engine == "postgresql" {
			return "DOUBLE PRECISION"
		}
		return "DOUBLE"
	case "text":
		return "TEXT"
	case "longtext":
		if engine == "postgresql" {
			return "TEXT"
		}
		return "LONGTEXT"
	case "boolean", "bool":
		if engine == "postgresql" {
			return "BOOLEAN"
		}
		return "TINYINT(1)"
	case "json":
		if engine == "postgresql" {
			return "JSONB"
		}
		return "JSON"
	case "uuid":
		if engine == "postgresql" {
			return "UUID"
		}
		return "VARCHAR(36)"
	case "date":
		return "DATE"
	case "timestamp", "datetime":
		if engine == "postgresql" {
			return "TIMESTAMP WITH TIME ZONE"
		}
		return "DATETIME"
	default:
		return "VARCHAR(255)"
	}
}

// ListBackups fetches history catalog of database snapshot backups
func (s *DatabaseService) ListBackups(projectID uint) ([]models.DatabaseBackup, error) {
	var backups []models.DatabaseBackup
	err := s.db.Where("project_id = ?", projectID).Order("created_at DESC").Find(&backups).Error
	return backups, err
}

// CreateBackup performs database snapshot backup, compresses it, and caps retention at 5 entries.
func (s *DatabaseService) CreateBackup(projectID uint) (*models.DatabaseBackup, error) {
	var project models.Project
	if err := s.db.Preload("DatabaseInstance").First(&project, projectID).Error; err != nil {
		return nil, err
	}

	if project.DatabaseInstance == nil {
		return nil, apperr.New(404, "DATABASE_NOT_ENABLED", "No database instance found for this project")
	}

	// Enforce naming
	timestamp := time.Now().Format("20060102_150405")
	backupName := fmt.Sprintf("%s_backup_%s.sql", project.DatabaseName, timestamp)

	// Check folders
	backupDir := filepath.Join(project.GetProjectPath(s.cfg.ProjectsPath), "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, err
	}
	backupPath := filepath.Join(backupDir, backupName)

	// Register pending record
	backup := &models.DatabaseBackup{
		DatabaseInstanceID: project.DatabaseInstance.ID,
		ProjectID:          project.ID,
		Name:               backupName,
		Path:               backupPath,
		Size:               "0 KB",
		Status:             models.BackupStatusPending,
		CreatedAt:          time.Now(),
	}
	if err := s.db.Create(backup).Error; err != nil {
		return nil, err
	}

	// Run backup dump
	dumpContent, err := s.GenerateProjectDump(project.DatabaseName, project.DatabasePassword)
	if err != nil {
		backup.Status = models.BackupStatusFailed
		s.db.Save(backup)
		return nil, fmt.Errorf("backup dump generation failed: %w", err)
	}

	// Write file
	if err := os.WriteFile(backupPath, []byte(dumpContent), 0644); err != nil {
		backup.Status = models.BackupStatusFailed
		s.db.Save(backup)
		return nil, fmt.Errorf("failed to save backup file: %w", err)
	}

	// Read physical file size
	fi, err := os.Stat(backupPath)
	sizeStr := "0 KB"
	if err == nil {
		kb := float64(fi.Size()) / 1024.0
		sizeStr = fmt.Sprintf("%.2f KB", kb)
		if kb > 1024 {
			sizeStr = fmt.Sprintf("%.2f MB", kb/1024.0)
		}
	}

	backup.Size = sizeStr
	backup.Status = models.BackupStatusCompleted
	s.db.Save(backup)

	// SRE CAP Policy: Enforce maximum limit of 5 backups per database
	var count int64
	s.db.Model(&models.DatabaseBackup{}).Where("database_instance_id = ? AND status = ?", project.DatabaseInstance.ID, models.BackupStatusCompleted).Count(&count)
	if count > 5 {
		var oldBackups []models.DatabaseBackup
		// Fetch excess old backups
		s.db.Where("database_instance_id = ? AND status = ?", project.DatabaseInstance.ID, models.BackupStatusCompleted).
			Order("created_at ASC").Limit(int(count - 5)).Find(&oldBackups)

		for _, ob := range oldBackups {
			_ = os.Remove(ob.Path)
			s.db.Delete(&ob)
			slog.Info("Pruned historical database backup due to 5-backup catalog retention limit", "name", ob.Name)
		}
	}

	return backup, nil
}

// GetBackupByID retrieves a specific database backup record for a project
func (s *DatabaseService) GetBackupByID(projectID uint, backupID uint) (*models.DatabaseBackup, error) {
	var backup models.DatabaseBackup
	if err := s.db.Where("id = ? AND project_id = ?", backupID, projectID).First(&backup).Error; err != nil {
		return nil, err
	}
	return &backup, nil
}

// RestoreBackup recovers SQL backup state into the project database
func (s *DatabaseService) RestoreBackup(projectID uint, backupID uint) error {
	var project models.Project
	if err := s.db.First(&project, projectID).Error; err != nil {
		return err
	}

	var backup models.DatabaseBackup
	if err := s.db.Where("id = ? AND project_id = ?", backupID, projectID).First(&backup).Error; err != nil {
		return err
	}

	sqlBytes, err := os.ReadFile(backup.Path)
	if err != nil {
		return fmt.Errorf("unable to read backup file: %w", err)
	}

	// Reset database tables
	_, err = s.ResetProjectDatabase(project.DatabaseName, project.DatabasePassword)
	if err != nil {
		return fmt.Errorf("failed to reset database before restore: %w", err)
	}

	// Execute restore
	statements := strings.Split(string(sqlBytes), ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		_, err = s.ExecuteRawQuery(project.DatabaseName, project.DatabasePassword, stmt)
		if err != nil {
			slog.Warn("Failed executing restore SQL statement", "database", project.DatabaseName, "error", err.Error())
		}
	}

	return nil
}

// DeleteBackup prunes a database backup physically and logically inside an atomic transaction.
func (s *DatabaseService) DeleteBackup(projectID uint, backupID uint) error {
	var backup models.DatabaseBackup
	if err := s.db.Where("id = ? AND project_id = ?", backupID, projectID).First(&backup).Error; err != nil {
		return err
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&backup).Error; err != nil {
			return err
		}
		if err := os.Remove(backup.Path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	})
	return err
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
	val = strings.ReplaceAll(val, "'", "''")
	val = strings.ReplaceAll(val, "\n", "\\n")
	val = strings.ReplaceAll(val, "\r", "\\r")
	val = strings.ReplaceAll(val, "\u0000", "\\0")
	val = strings.ReplaceAll(val, "\u001a", "\\Z")
	val = strings.ReplaceAll(val, "\u0008", "\\b")
	return val
}

func (s *DatabaseService) escapeIdentifier(engine, name string) string {
	if engine == "postgresql" {
		return fmt.Sprintf("\"%s\"", strings.ReplaceAll(name, "\"", "\"\""))
	}
	return fmt.Sprintf("`%s`", strings.ReplaceAll(name, "`", "``"))
}

// GetAllSchemaMetadata fetches columns and foreign keys for all tables in a single step to prevent N+1 query loops.
func (s *DatabaseService) GetAllSchemaMetadata(dbName, password string) (map[string][]ColumnInfo, map[string][]ForeignKeyInfo, error) {
	db, err := s.ConnectToProjectDB(dbName, password)
	if err != nil {
		return nil, nil, err
	}

	engine := s.getEngineForDB(dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	columnsMap := make(map[string][]ColumnInfo)
	fksMap := make(map[string][]ForeignKeyInfo)

	// 1. Fetch All Columns
	var colRows *sql.Rows
	if engine == "postgresql" {
		colRows, err = db.QueryContext(ctx, `
			SELECT 
				c.table_name,
				c.column_name,
				c.data_type,
				c.is_nullable,
				COALESCE(
					(SELECT 'PRI' FROM pg_index i 
					 JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
					 WHERE i.indrelid = (c.table_schema || '.' || c.table_name)::regclass AND i.indisprimary AND a.attname = c.column_name LIMIT 1), 
					''
				) AS column_key,
				c.column_default,
				'' AS extra,
				COALESCE(pg_catalog.col_description((c.table_schema || '.' || c.table_name)::regclass, c.ordinal_position), '') AS column_comment
			FROM information_schema.columns c
			WHERE c.table_schema = 'public'
			ORDER BY c.table_name, c.ordinal_position
		`)
	} else {
		colRows, err = db.QueryContext(ctx, `
			SELECT 
				TABLE_NAME,
				COLUMN_NAME,
				COLUMN_TYPE,
				IS_NULLABLE,
				COLUMN_KEY,
				COLUMN_DEFAULT,
				EXTRA,
				COLUMN_COMMENT
			FROM information_schema.COLUMNS 
			WHERE TABLE_SCHEMA = ?
			ORDER BY TABLE_NAME, ORDINAL_POSITION
		`, dbName)
	}

	if err != nil {
		return nil, nil, err
	}
	defer colRows.Close()

	for colRows.Next() {
		var tableName string
		var col ColumnInfo
		var nullable string
		if err := colRows.Scan(&tableName, &col.Name, &col.Type, &nullable, &col.Key, &col.Default, &col.Extra, &col.Comment); err != nil {
			continue
		}
		col.Nullable = (nullable == "YES" || nullable == "yes" || nullable == "TRUE" || nullable == "true")
		columnsMap[tableName] = append(columnsMap[tableName], col)
	}
	if err = colRows.Err(); err != nil {
		return nil, nil, err
	}

	// 2. Fetch All Foreign Keys
	var fkRows *sql.Rows
	if engine == "postgresql" {
		fkRows, err = db.QueryContext(ctx, `
			SELECT
				tc.table_name,
				kcu.column_name,
				ccu.table_name AS referenced_table_name,
				ccu.column_name AS referenced_column_name,
				tc.constraint_name
			FROM
				information_schema.table_constraints AS tc
				JOIN information_schema.key_column_usage AS kcu
				  ON tc.constraint_name = kcu.constraint_name
				  AND tc.table_schema = kcu.table_schema
				JOIN information_schema.constraint_column_usage AS ccu
				  ON ccu.constraint_name = tc.constraint_name
				  AND ccu.table_schema = tc.table_schema
			WHERE 
				tc.constraint_type = 'FOREIGN KEY' 
				AND tc.table_schema = 'public';
		`)
	} else {
		fkRows, err = db.QueryContext(ctx, `
			SELECT 
				TABLE_NAME,
				COLUMN_NAME,
				REFERENCED_TABLE_NAME,
				REFERENCED_COLUMN_NAME,
				CONSTRAINT_NAME
			FROM 
				INFORMATION_SCHEMA.KEY_COLUMN_USAGE
			WHERE 
				TABLE_SCHEMA = ? 
				AND REFERENCED_TABLE_NAME IS NOT NULL;
		`, dbName)
	}

	if err != nil {
		// Log error internally but do not fail the whole load; return empty FK map to degrade gracefully
		return columnsMap, fksMap, nil
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var tableName string
		var fk ForeignKeyInfo
		if err := fkRows.Scan(&tableName, &fk.ColumnName, &fk.TargetTable, &fk.TargetColumn, &fk.ConstraintName); err != nil {
			continue
		}
		fksMap[tableName] = append(fksMap[tableName], fk)
	}
	if err = fkRows.Err(); err != nil {
		return columnsMap, fksMap, nil
	}

	return columnsMap, fksMap, nil
}

// AdminListAllDatabases returns a summary of all user databases
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
		engine := s.getEngineForDB(p.DatabaseName)
		info := AdminDatabaseInfo{
			ProjectID:    p.ID,
			ProjectName:  p.Name,
			UserName:     p.UserName,
			DatabaseName: p.DatabaseName,
			Engine:       engine,
			Status:       p.Status,
			TableCount:   0,
			Size:         "0 KB",
		}

		db, err := s.ConnectToProjectDB(p.DatabaseName, p.DatabasePassword)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			var tableCount int
			var totalSize float64

			if engine == "postgresql" {
				// PostgreSQL size queries
				_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM pg_tables WHERE schemaname = 'public'").Scan(&tableCount)
				_ = db.QueryRowContext(ctx, "SELECT COALESCE(SUM(pg_total_relation_size(quote_ident(tablename)) / 1024.0), 0) FROM pg_tables WHERE schemaname = 'public'").Scan(&totalSize)
			} else {
				// MySQL size queries
				_ = db.QueryRowContext(ctx, `
					SELECT 
						COUNT(*),
						COALESCE(SUM(DATA_LENGTH + INDEX_LENGTH) / 1024.0, 0)
					FROM information_schema.TABLES 
					WHERE TABLE_SCHEMA = ?
				`, p.DatabaseName).Scan(&tableCount, &totalSize)
			}
			cancel()

			info.TableCount = tableCount
			info.Size = fmt.Sprintf("%.2f KB", totalSize)
			if totalSize > 1024 {
				info.Size = fmt.Sprintf("%.2f MB", totalSize/1024.0)
			}
		}

		result = append(result, info)
	}

	return result, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// SplitSQLStatements splits a SQL string into individual statements by semicolon,
// respecting single-quoted strings, double-quoted strings, and SQL comments.
func SplitSQLStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false
	length := len(sql)

	for i := 0; i < length; i++ {
		ch := sql[i]

		// Handle comments
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				current.WriteByte(ch)
			} else {
				current.WriteByte(ch)
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < length && sql[i+1] == '/' {
				inBlockComment = false
				current.WriteByte('*')
				current.WriteByte('/')
				i++ // Skip '/'
			} else {
				current.WriteByte(ch)
			}
			continue
		}

		// Check for comments start
		if !inSingleQuote && !inDoubleQuote {
			if ch == '-' && i+1 < length && sql[i+1] == '-' {
				inLineComment = true
				current.WriteByte('-')
				current.WriteByte('-')
				i++
				continue
			}
			if ch == '/' && i+1 < length && sql[i+1] == '*' {
				inBlockComment = true
				current.WriteByte('/')
				current.WriteByte('*')
				i++
				continue
			}
		}

		// Handle string quotes
		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
			current.WriteByte(ch)
			continue
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
			current.WriteByte(ch)
			continue
		}

		// Handle semicolon
		if ch == ';' && !inSingleQuote && !inDoubleQuote {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	stmt := strings.TrimSpace(current.String())
	if stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// normalizeSQLForCheck strips comments, removes spaces around dots, normalizes other whitespaces,
// and returns the UPPERCASE query for secure block list matching.
func normalizeSQLForCheck(query string) string {
	var result strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false
	length := len(query)

	for i := 0; i < length; i++ {
		ch := query[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				result.WriteByte(' ') // Replace comment line with a space
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < length && query[i+1] == '/' {
				inBlockComment = false
				result.WriteByte(' ') // Replace block comment with a space
				i++
			}
			continue
		}

		if !inSingleQuote && !inDoubleQuote {
			if ch == '-' && i+1 < length && query[i+1] == '-' {
				inLineComment = true
				i++
				continue
			}
			if ch == '/' && i+1 < length && query[i+1] == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
		}
		if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		}

		result.WriteByte(ch)
	}

	// Convert to UPPERCASE
	upper := strings.ToUpper(result.String())

	// Strip spaces around dots '.' to prevent whitespace obfuscation like 'mysql . user'
	for strings.Contains(upper, " .") || strings.Contains(upper, ". ") {
		upper = strings.ReplaceAll(upper, " .", ".")
		upper = strings.ReplaceAll(upper, ". ", ".")
	}

	// Normalize whitespaces: replace tabs, newlines, and multiple spaces with a single space
	var normalized strings.Builder
	lastWasSpace := false
	for _, r := range upper {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !lastWasSpace {
				normalized.WriteRune(' ')
				lastWasSpace = true
			}
		} else {
			normalized.WriteRune(r)
			lastWasSpace = false
		}
	}

	return strings.TrimSpace(normalized.String())
}
