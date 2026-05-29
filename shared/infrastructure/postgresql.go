package infrastructure

import (
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/pkg/utils"
)

var (
	validDBNameRegex   = regexp.MustCompile(`^[a-z0-9_]{3,63}$`)
	validPasswordRegex = regexp.MustCompile(`^[a-zA-Z0-9]{8,128}$`)
)

// PostgreSQLService handles PostgreSQL database provisioning for user projects
// inside the dedicated paas-user-postgres container (isolated from the control plane).
type PostgreSQLService struct{}

func NewPostgreSQLService() *PostgreSQLService {
	return &PostgreSQLService{}
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

	// Create role with connection limit to prevent tenant connection storms and SQL injection
	createRoleSQL := fmt.Sprintf(
		"CREATE ROLE \"%s\" WITH LOGIN PASSWORD '%s' CONNECTION LIMIT 15;",
		dbName, password,
	)
	res, err := utils.Run(1*time.Minute, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", createRoleSQL)
	if err != nil {
		return apperr.New(500, "PG_ROLE_FAILED", "Failed to create PostgreSQL role: "+res.Stderr)
	}

	// Create database owned by the new role
	createDBSQL := fmt.Sprintf(
		"CREATE DATABASE \"%s\" OWNER \"%s\";",
		dbName, dbName,
	)
	res, err = utils.Run(1*time.Minute, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", createDBSQL)
	if err != nil {
		return apperr.New(500, "PG_DB_FAILED", "Failed to create PostgreSQL database: "+res.Stderr)
	}

	// Apply idle connection cleanup timeouts to prevent hung sessions from leaking RAM
	idleSQL := fmt.Sprintf(
		"ALTER DATABASE \"%s\" SET idle_in_transaction_session_timeout = '60000'; ALTER DATABASE \"%s\" SET idle_session_timeout = '300000';",
		dbName, dbName,
	)
	if res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", idleSQL); err != nil {
		slog.Warn("Failed to configure idle connection timeouts", "db", dbName, "err", err, "stderr", res.Stderr)
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
	if res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", terminateSQL); err != nil {
		slog.Warn("Failed to terminate active connections before drop", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS \"%s\";", dbName)
	if res, err := utils.Run(1*time.Minute, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", dropDBSQL); err != nil {
		slog.Warn("Failed to drop PostgreSQL database", "db", dbName, "err", err, "stderr", res.Stderr)
	}

	dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS \"%s\";", dbName)
	if res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", dropRoleSQL); err != nil {
		slog.Warn("Failed to drop PostgreSQL role", "role", dbName, "err", err, "stderr", res.Stderr)
	}

	return nil
}

// UpdatePassword rotates the database user's password inside the PostgreSQL engine.
func (s *PostgreSQLService) UpdatePassword(dbName, newPassword string) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validPasswordRegex.MatchString(newPassword) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must contain only alphanumeric characters and be 8-128 characters long")
	}

	alterSQL := fmt.Sprintf("ALTER ROLE \"%s\" WITH PASSWORD '%s';", dbName, newPassword)
	res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
		"psql", "-U", "postgres", "-c", alterSQL)
	if err != nil {
		return apperr.New(500, "PG_PASSWORD_FAILED", "Failed to update PostgreSQL password: "+res.Stderr)
	}
	return nil
}

// UpdateStatus suspends or resumes a database by revoking/granting CONNECT privileges.
// Suspension also terminates all active backend connections to enforce immediate lockout.
func (s *PostgreSQLService) UpdateStatus(dbName string, suspend bool) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}

	if suspend {
		revokeSQL := fmt.Sprintf("REVOKE CONNECT ON DATABASE \"%s\" FROM \"%s\";", dbName, dbName)
		res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
			"psql", "-U", "postgres", "-c", revokeSQL)
		if err != nil {
			return apperr.New(500, "PG_SUSPEND_FAILED", "Failed to revoke connect: "+res.Stderr)
		}

		// Terminate all active connections to enforce immediate lockout
		terminateSQL := fmt.Sprintf(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();",
			dbName,
		)
		if res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
			"psql", "-U", "postgres", "-c", terminateSQL); err != nil {
			slog.Warn("Failed to terminate active connections during suspend", "db", dbName, "err", err, "stderr", res.Stderr)
		}
	} else {
		grantSQL := fmt.Sprintf("GRANT CONNECT ON DATABASE \"%s\" TO \"%s\";", dbName, dbName)
		res, err := utils.Run(30*time.Second, "docker", "exec", "paas-user-postgres",
			"psql", "-U", "postgres", "-c", grantSQL)
		if err != nil {
			return apperr.New(500, "PG_RESUME_FAILED", "Failed to grant connect: "+res.Stderr)
		}
	}

	return nil
}
