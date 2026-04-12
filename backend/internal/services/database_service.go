// ===========================================
// Database Service
// ===========================================
// Orhcestrates project database operations
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

// ConnectToProjectDB returns a pooled connection to a student's MySQL database.
// Connections are cached and reused across requests to avoid connection storms.
func (s *DatabaseService) ConnectToProjectDB(dbName string) (*sql.DB, error) {
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

	dsn := fmt.Sprintf("%s:%s@tcp(paas-mysql:3306)/%s?parseTime=true",
		dbName, dbName, dbName,
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
func (s *DatabaseService) ListProjectTables(dbName string) ([]TableInfo, error) {
	db, err := s.ConnectToProjectDB(dbName)
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
func (s *DatabaseService) GetTableStructure(dbName, tableName string) ([]ColumnInfo, error) {
	db, err := s.ConnectToProjectDB(dbName)
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
func (s *DatabaseService) GetTableData(dbName, tableName string, page, limit int) ([]string, []map[string]interface{}, int64, error) {
	if !s.isValidIdentifier(tableName) {
		return nil, nil, 0, apperr.New(400, "INVALID_TABLE_NAME", "Table name contains invalid characters or exceeds length limit")
	}

	db, err := s.ConnectToProjectDB(dbName)
	if err != nil {
		return nil, nil, 0, err
	}


	var total int64
	db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", tableName)).Scan(&total)

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

	return columns, data, total, nil
}

// ExecuteRawQuery runs a manual SQL query against a project database
func (s *DatabaseService) ExecuteRawQuery(dbName, query string) (*QueryResult, error) {
	query = strings.TrimSpace(query)
	upperQuery := strings.ToUpper(query)

	if strings.Contains(upperQuery, "DROP DATABASE") ||
		strings.Contains(upperQuery, "CREATE DATABASE") ||
		strings.Contains(upperQuery, "GRANT") ||
		strings.Contains(upperQuery, "REVOKE") {
		slog.Warn("Blocked forbidden SQL operation attempt", "query", upperQuery[:min(len(upperQuery), 80)])
		return nil, apperr.New(403, "SQL_OPERATION_FORBIDDEN", "This SQL operation is not permitted for security reasons")
	}

	db, err := s.ConnectToProjectDB(dbName)
	if err != nil {
		return nil, err
	}


	start := time.Now()

	// Handle data retrieval queries
	if strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "SHOW") || strings.HasPrefix(upperQuery, "DESCRIBE") {
		rows, err := db.Query(query)
		if err != nil {
			return nil, err
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

		return &QueryResult{
			Columns:  columns,
			Rows:     data,
			Duration: time.Since(start).String(),
		}, nil
	}

	// Handle modification queries
	result, err := db.Exec(query)
	if err != nil {
		return nil, err
	}

	affected, _ := result.RowsAffected()
	return &QueryResult{
		RowsAffected: affected,
		Duration:     time.Since(start).String(),
	}, nil
}

// GenerateProjectDump creates a logical SQL export for a project database
func (s *DatabaseService) GenerateProjectDump(dbName string) (string, error) {
	db, err := s.ConnectToProjectDB(dbName)
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
		tables.Scan(&tableName)

		var tbl, createStmt string
		db.QueryRow(fmt.Sprintf("SHOW CREATE TABLE `%s`", tableName)).Scan(&tbl, &createStmt)
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
			rows.Scan(valuePtrs...)

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
func (s *DatabaseService) ResetProjectDatabase(dbName string) (int, error) {
	db, err := s.ConnectToProjectDB(dbName)
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

	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	for _, table := range tables {
		db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS `%s`", table))
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")

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
