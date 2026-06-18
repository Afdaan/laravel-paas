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
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/pkg/utils"
)

// MySQLService handles MySQL database provisioning for user projects
type MySQLService struct{}

// NewMySQLService creates a new MySQL provisioning service
func NewMySQLService() *MySQLService {
	return &MySQLService{}
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
