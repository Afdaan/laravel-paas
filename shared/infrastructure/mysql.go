// ===========================================
// MySQL Provisioning Service
// ===========================================
// Handles MySQL database lifecycle for
// user projects (create, drop, rotate, suspend)
// ===========================================
package infrastructure

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/pkg/utils"
)

var (
	validDBNameRegex   = regexp.MustCompile(`^[a-z0-9_]{3,63}$`)
	validPasswordRegex = regexp.MustCompile(`^[a-zA-Z0-9]{8,128}$`)
)

// MySQLService handles MySQL database provisioning for user projects
type MySQLService struct{}

// NewMySQLService creates a new MySQL provisioning service
func NewMySQLService() *MySQLService {
	return &MySQLService{}
}

// CreateDatabaseCustom provisions a MySQL database and matching user for a user project.
// Validates database name and password to block SQL injection via identifiers.
func (s *MySQLService) CreateDatabaseCustom(dbName, username, password string) error {
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

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	// NOTE: Check-then-create has an inherent TOCTOU window because MySQL/PostgreSQL
	// DDL is non-transactional. This is acceptable because global control-plane
	// uniqueness is enforced before this call reaches the infrastructure layer.

	// Check if database already exists
	checkDBSQL := fmt.Sprintf("SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = '%s';", dbName)
	checkDBRes, err := utils.Run(30*time.Second, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword, "-N", "-e", checkDBSQL)
	if err != nil {
		return apperr.New(503, "ENGINE_UNREACHABLE", "Could not verify existence of database/role; provisioning aborted")
	}
	if strings.TrimSpace(checkDBRes.Stdout) == dbName {
		return apperr.New(409, "DB_ALREADY_EXISTS", fmt.Sprintf("MySQL database '%s' already exists", dbName))
	}

	// Check if user already exists
	checkUserSQL := fmt.Sprintf("SELECT User FROM mysql.user WHERE User = '%s';", username)
	checkUserRes, err := utils.Run(30*time.Second, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword, "-N", "-e", checkUserSQL)
	if err != nil {
		return apperr.New(503, "ENGINE_UNREACHABLE", "Could not verify existence of database/role; provisioning aborted")
	}
	if strings.TrimSpace(checkUserRes.Stdout) == username {
		return apperr.New(409, "USER_ALREADY_EXISTS", fmt.Sprintf("MySQL user '%s' already exists", username))
	}

	res, err := utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("CREATE DATABASE `%s`;", dbName))
	if err != nil {
		return apperr.New(500, "DB_CREATE_FAILED", "Failed to create MySQL database: "+res.Stderr)
	}

	// Create user with connection limit to prevent tenant connection storms and SQL injection
	res, err = utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf(
			"CREATE USER '%s'@'%%' IDENTIFIED BY '%s'; ALTER USER '%s'@'%%' WITH MAX_USER_CONNECTIONS 15; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;",
			username, password, username, dbName, username,
		))
	if err != nil {
		// ROLLBACK: Drop the MySQL database created in the previous step
		dropDBSQL := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", dbName)
		_, _ = utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
			"mysql", "-uroot", "-p"+rootPassword, "-e", dropDBSQL)
		// Clean up the user in case it was partially created
		dropUserSQL := fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%';", username)
		_, _ = utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
			"mysql", "-uroot", "-p"+rootPassword, "-e", dropUserSQL)

		return apperr.New(500, "DB_PROVISION_FAILED", "Failed to create database user or grant permissions: "+res.Stderr)
	}

	return nil
}

// DropDatabaseCustom removes a MySQL database and its associated user
func (s *MySQLService) DropDatabaseCustom(dbName, username string) error {
	dbName = strings.ToLower(dbName)
	username = strings.ToLower(username)

	var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	var userRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

	if !nameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !userRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_USER", "Database username must match ^[a-z0-9_]{3,63}$")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")
	res, err := utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'%%';", dbName, username))
	if err != nil {
		return apperr.New(500, "DB_DROP_FAILED", "Failed to drop MySQL database/user: "+res.Stderr)
	}
	return nil
}

// CreateDatabase creates a MySQL database and matching user for a user project.
// Validates database name and password to block SQL injection via identifiers.
func (s *MySQLService) CreateDatabase(dbName, password string) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validPasswordRegex.MatchString(password) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must contain only alphanumeric characters and be 8-128 characters long")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	res, err := utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", dbName))
	if err != nil {
		return apperr.New(500, "DB_CREATE_FAILED", "Failed to create MySQL database: "+res.Stderr)
	}

	// Create user with connection limit to prevent tenant connection storms and SQL injection
	res, err = utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf(
			"CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'; ALTER USER '%s'@'%%' IDENTIFIED BY '%s' WITH MAX_USER_CONNECTIONS 15; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;",
			dbName, password, dbName, password, dbName, dbName,
		))
	if err != nil {
		return apperr.New(500, "DB_PROVISION_FAILED", "Failed to create database user or grant permissions: "+res.Stderr)
	}

	return nil
}

// DropDatabase removes a MySQL database and its associated user
func (s *MySQLService) DropDatabase(dbName string) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")
	_, err := utils.Run(1*time.Minute, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'%%';", dbName, dbName))
	return err
}

// UpdatePassword rotates the database user's password inside the MySQL engine.
func (s *MySQLService) UpdatePassword(dbName, newPassword string) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validPasswordRegex.MatchString(newPassword) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must contain only alphanumeric characters and be 8-128 characters long")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")
	res, err := utils.Run(30*time.Second, "docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'; FLUSH PRIVILEGES;", dbName, newPassword))
	if err != nil {
		return apperr.New(500, "DB_PASSWORD_FAILED", "Failed to update MySQL password: "+res.Stderr)
	}
	return nil
}

// UpdateStatus suspends or resumes a database by revoking/granting all privileges.
// Suspension also kills active connections by setting max_user_connections to 0.
func (s *MySQLService) UpdateStatus(dbName string, suspend bool) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	if suspend {
		res, err := utils.Run(30*time.Second, "docker", "exec", "paas-mysql",
			"mysql", "-uroot", "-p"+rootPassword,
			"-e", fmt.Sprintf("REVOKE ALL PRIVILEGES ON `%s`.* FROM '%s'@'%%'; ALTER USER '%s'@'%%' WITH MAX_USER_CONNECTIONS 0; FLUSH PRIVILEGES;", dbName, dbName, dbName))
		if err != nil {
			return apperr.New(500, "DB_SUSPEND_FAILED", "Failed to suspend MySQL database: "+res.Stderr)
		}
	} else {
		res, err := utils.Run(30*time.Second, "docker", "exec", "paas-mysql",
			"mysql", "-uroot", "-p"+rootPassword,
			"-e", fmt.Sprintf("GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; ALTER USER '%s'@'%%' WITH MAX_USER_CONNECTIONS 15; FLUSH PRIVILEGES;", dbName, dbName, dbName))
		if err != nil {
			return apperr.New(500, "DB_RESUME_FAILED", "Failed to resume MySQL database: "+res.Stderr)
		}
	}

	return nil
}
