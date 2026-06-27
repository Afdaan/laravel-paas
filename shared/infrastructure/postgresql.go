package infrastructure

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/pkg/utils"
)

// validDBNameRegex and validPasswordRegex are defined in mysql.go to avoid redeclaration in the same package.

// PostgreSQLContainerName returns the container name for PostgreSQL, checking POSTGRES_CONTAINER_NAME env var with fallback
func PostgreSQLContainerName() string {
	if name := os.Getenv("POSTGRES_CONTAINER_NAME"); name != "" {
		return name
	}
	return "paas-user-postgres"
}

// PostgreSQLPort returns the configured user PostgreSQL port with the current default fallback.
func PostgreSQLPort() int {
	if os.Getenv("APP_MODE") == "docker" {
		return 5432
	}
	return databasePort("USER_PG_PORT", 5433)
}

// PostgreSQLService handles PostgreSQL database provisioning for user projects
// inside the dedicated paas-user-postgres container (isolated from the control plane).
type PostgreSQLService struct {
	containerName string
}

func NewPostgreSQLService() *PostgreSQLService {
	return &PostgreSQLService{
		containerName: PostgreSQLContainerName(),
	}
}

// CreateDatabase provisions a new PostgreSQL database and role with SRE-hardened defaults.
// Enforces strict naming validation, password security validation, connection limits, and idle timeouts.
func (s *PostgreSQLService) CreateDatabase(dbName, password string) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validPasswordRegex.MatchString(password) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must contain only alphanumeric characters and be 8-128 characters long")
	}

	// Check if role already exists
	checkRoleSQL := fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname = '%s';", dbName)
	checkRoleRes, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-t", "-c", checkRoleSQL)

	roleExists := err == nil && strings.Contains(checkRoleRes.Stdout, "1")

	if !roleExists {
		// Create role with connection limit to prevent tenant connection storms and SQL injection
		createRoleSQL := fmt.Sprintf(
			"CREATE ROLE \"%s\" WITH LOGIN PASSWORD '%s' CONNECTION LIMIT 15;",
			dbName, password,
		)
		res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", createRoleSQL)
		if err != nil {
			return apperr.New(500, "PG_ROLE_FAILED", "Failed to create PostgreSQL role: "+res.Stderr)
		}
	} else {
		// If role exists, safely update/rotate its password to ensure it matches the current session
		alterRoleSQL := fmt.Sprintf("ALTER ROLE \"%s\" WITH PASSWORD '%s';", dbName, password)
		if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", alterRoleSQL); err != nil {
			slog.Warn("Failed to update password for existing PostgreSQL role", "role", dbName, "err", err, "stderr", res.Stderr)
		}
	}

	// Check if database already exists
	checkDBSQL := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s';", dbName)
	checkDBRes, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-t", "-c", checkDBSQL)

	dbExists := err == nil && strings.Contains(checkDBRes.Stdout, "1")

	if !dbExists {
		// Create database owned by the new role
		createDBSQL := fmt.Sprintf(
			"CREATE DATABASE \"%s\" OWNER \"%s\";",
			dbName, dbName,
		)
		res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", createDBSQL)
		if err != nil {
			return apperr.New(500, "PG_DB_FAILED", "Failed to create PostgreSQL database: "+res.Stderr)
		}
	}

	// Apply idle connection cleanup timeouts to prevent hung sessions from leaking RAM
	idleSQL := fmt.Sprintf(
		"ALTER DATABASE \"%s\" SET idle_in_transaction_session_timeout = '60000'; ALTER DATABASE \"%s\" SET idle_session_timeout = '300000';",
		dbName, dbName,
	)
	if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", idleSQL); err != nil {
		slog.Warn("Failed to configure idle connection timeouts", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	return nil
}

// CreateDatabaseCustom provisions a new PostgreSQL database and a distinct role with SRE-hardened defaults.
// Enforces naming validation, password validation, connection limits, and idle timeouts.
func (s *PostgreSQLService) CreateDatabaseCustom(dbName, username, password string) error {
	// Enforce lowercase names for DB and username
	dbName = strings.ToLower(dbName)
	username = strings.ToLower(username)

	var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	var userRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	var passRegex = regexp.MustCompile(`^[^[:space:]"'\x60\\;@#/?]{12,128}$`)

	if !nameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must be 2-64 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if !userRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_USER", "Database username must be 2-32 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if !passRegex.MatchString(password) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must be 12-128 characters and not contain invalid characters")
	}

	// NOTE: Check-then-create has an inherent TOCTOU window because MySQL/PostgreSQL
	// DDL is non-transactional. This is acceptable because global control-plane
	// uniqueness is enforced before this call reaches the infrastructure layer.

	// Check if role already exists
	checkRoleSQL := fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname = '%s';", username)
	checkRoleRes, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-t", "-c", checkRoleSQL)
	if err != nil {
		return apperr.New(503, "ENGINE_UNREACHABLE", "Could not verify existence of database/role; provisioning aborted")
	}

	roleExists := strings.Contains(checkRoleRes.Stdout, "1")

	if roleExists {
		return apperr.New(409, "ROLE_ALREADY_EXISTS", fmt.Sprintf("PostgreSQL role '%s' already exists", username))
	}

	// Create role with connection limit to prevent tenant connection storms and SQL injection
	createRoleSQL := fmt.Sprintf(
		"CREATE ROLE \"%s\" WITH LOGIN PASSWORD '%s' CONNECTION LIMIT 15;",
		username, password,
	)
	res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", createRoleSQL)
	if err != nil {
		return apperr.New(500, "PG_ROLE_FAILED", "Failed to create PostgreSQL role: "+res.Stderr)
	}

	// NOTE: Check-then-create has an inherent TOCTOU window because MySQL/PostgreSQL
	// DDL is non-transactional. This is acceptable because global control-plane
	// uniqueness is enforced before this call reaches the infrastructure layer.

	// Check if database already exists
	checkDBSQL := fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = '%s';", dbName)
	checkDBRes, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-t", "-c", checkDBSQL)
	if err != nil {
		// ROLLBACK: Drop PostgreSQL role created in previous step
		dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", username)
		_, _ = utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", dropRoleSQL)
		return apperr.New(503, "ENGINE_UNREACHABLE", "Could not verify existence of database/role; provisioning aborted")
	}

	dbExists := strings.Contains(checkDBRes.Stdout, "1")

	if dbExists {
		// ROLLBACK: Drop PostgreSQL role created in previous step
		dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", username)
		_, _ = utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", dropRoleSQL)
		return apperr.New(409, "DB_ALREADY_EXISTS", fmt.Sprintf("PostgreSQL database '%s' already exists", dbName))
	}

	// Create database owned by the new role
	createDBSQL := fmt.Sprintf(
		"CREATE DATABASE \"%s\" OWNER \"%s\";",
		dbName, username,
	)
	res, err = utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", createDBSQL)
	if err != nil {
		// ROLLBACK: Drop PostgreSQL role created in previous step
		dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", username)
		_, _ = utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", dropRoleSQL)
		return apperr.New(500, "PG_DB_FAILED", "Failed to create PostgreSQL database: "+res.Stderr)
	}

	// Apply idle connection cleanup timeouts to prevent hung sessions from leaking RAM
	idleSQL := fmt.Sprintf(
		"ALTER DATABASE \"%s\" SET idle_in_transaction_session_timeout = '60000'; ALTER DATABASE \"%s\" SET idle_session_timeout = '300000';",
		dbName, dbName,
	)
	if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", idleSQL); err != nil {
		slog.Warn("Failed to configure idle connection timeouts", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	return nil
}

// DropDatabaseCustom drops database and custom role.
func (s *PostgreSQLService) DropDatabaseCustom(dbName, username string) error {
	dbName = strings.ToLower(dbName)
	username = strings.ToLower(username)

	var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	var userRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

	if !nameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must be 2-64 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if !userRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_USER", "Database username must be 2-32 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}

	// Force-disconnect all active sessions before dropping
	terminateSQL := fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
		dbName,
	)
	if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", terminateSQL); err != nil {
		slog.Warn("Failed to terminate active connections before drop", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\";", dbName)
	if res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", dropDBSQL); err != nil {
		return apperr.New(500, "PG_DROP_DB_FAILED", "Failed to drop PostgreSQL database: "+res.Stderr)
	}

	dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", username)
	if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", dropRoleSQL); err != nil {
		return apperr.New(500, "PG_DROP_ROLE_FAILED", "Failed to drop PostgreSQL role: "+res.Stderr)
	}

	return nil
}

// DropDatabase gracefully terminates active sessions and drops both the database and role.
func (s *PostgreSQLService) DropDatabase(dbName string) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}

	// Force-disconnect all active sessions before dropping
	terminateSQL := fmt.Sprintf(
		"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
		dbName,
	)
	if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", terminateSQL); err != nil {
		slog.Warn("Failed to terminate active connections before drop", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\";", dbName)
	if res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", dropDBSQL); err != nil {
		slog.Warn("Failed to drop PostgreSQL database", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", dbName)
	if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", dropRoleSQL); err != nil {
		slog.Warn("Failed to drop PostgreSQL role", "role", dbName, "err", err, "stderr", res.Stderr)
	}

	return nil
}

// UpdatePassword rotates the database user's password inside the PostgreSQL engine.
func (s *PostgreSQLService) UpdatePassword(username, newPassword string) error {
	if !validDBNameRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validPasswordRegex.MatchString(newPassword) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must contain only alphanumeric characters and be 8-128 characters long")
	}

	alterSQL := fmt.Sprintf("ALTER ROLE \"%s\" WITH PASSWORD '%s';", username, newPassword)
	res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"psql", "-U", "postgres", "-c", alterSQL)
	if err != nil {
		return apperr.New(500, "PG_PASSWORD_FAILED", "Failed to update PostgreSQL password: "+res.Stderr)
	}
	return nil
}

// UpdateStatus suspends or resumes a database by revoking/granting CONNECT privileges.
// Suspension also terminates all active backend connections to enforce immediate lockout.
func (s *PostgreSQLService) UpdateStatus(dbName, username string, suspend bool) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validDBNameRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}

	if suspend {
		revokeSQL := fmt.Sprintf("REVOKE CONNECT ON DATABASE \"%s\" FROM \"%s\";", dbName, username)
		res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", revokeSQL)
		if err != nil {
			return apperr.New(500, "PG_SUSPEND_FAILED", "Failed to revoke connect: "+res.Stderr)
		}

		// Terminate all active connections to enforce immediate lockout
		terminateSQL := fmt.Sprintf(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
			dbName,
		)
		if res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", terminateSQL); err != nil {
			slog.Warn("Failed to terminate active connections during suspend", "db", dbName, "err", err, "stderr", res.Stderr)
		}
	} else {
		grantSQL := fmt.Sprintf("GRANT CONNECT ON DATABASE \"%s\" TO \"%s\";", dbName, username)
		res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"psql", "-U", "postgres", "-c", grantSQL)
		if err != nil {
			return apperr.New(500, "PG_RESUME_FAILED", "Failed to grant connect: "+res.Stderr)
		}
	}

	return nil
}
