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
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/pkg/utils"
)

var (
	validDBNameRegex   = regexp.MustCompile(`^[a-z0-9_]{3,63}$`)
	validPasswordRegex = regexp.MustCompile(`^[a-zA-Z0-9]{8,128}$`)
)

// MySQLContainerName returns the container name for MySQL, checking MYSQL_CONTAINER_NAME env var with fallback
func MySQLContainerName() string {
	if name := os.Getenv("MYSQL_CONTAINER_NAME"); name != "" {
		return name
	}
	return "paas-mysql"
}

func databasePort(envName string, fallback int) int {
	value := os.Getenv(envName)
	if value == "" {
		return fallback
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fallback
	}

	return port
}

// MySQLPort returns the configured MySQL port with the current default fallback.
func MySQLPort() int {
	return databasePort("MYSQL_PORT", 3306)
}

// MySQLService handles MySQL database provisioning for user projects
type MySQLService struct {
	containerName string
}

// ProvisioningOwnership identifies physical resources created by one attempt.
// Cleanup must act only on resources this attempt created.
type ProvisioningOwnership struct {
	DatabaseCreated bool
	UserCreated     bool
}

const DefaultManagedDatabaseConnectionLimit = 15

func (ownership ProvisioningOwnership) HasResources() bool {
	return ownership.DatabaseCreated || ownership.UserCreated
}

// NewMySQLService creates a new MySQL provisioning service
func NewMySQLService() *MySQLService {
	return &MySQLService{
		containerName: MySQLContainerName(),
	}
}

// CreateDatabaseCustom provisions a MySQL database and matching user for a user project.
// Validates database name and password to block SQL injection via identifiers.
func (s *MySQLService) CreateDatabaseCustom(dbName, username, password string) error {
	return s.CreateDatabaseCustomWithConnectionLimit(dbName, username, password, DefaultManagedDatabaseConnectionLimit)
}

func (s *MySQLService) CreateDatabaseCustomWithConnectionLimit(dbName, username, password string, connectionLimit int) error {
	ownership, err := s.ProvisionDatabaseCustomWithConnectionLimit(dbName, username, password, connectionLimit)
	if err != nil && ownership.HasResources() {
		if cleanupErr := s.DropDatabaseCustomOwned(dbName, username, ownership); cleanupErr != nil {
			return fmt.Errorf("%w; compensate provisioning: %v", err, cleanupErr)
		}
	}
	return err
}

// ProvisionDatabaseCustom reports exactly which physical resources it created.
func (s *MySQLService) ProvisionDatabaseCustom(dbName, username, password string) (ProvisioningOwnership, error) {
	return s.ProvisionDatabaseCustomWithConnectionLimit(dbName, username, password, DefaultManagedDatabaseConnectionLimit)
}

func (s *MySQLService) ProvisionDatabaseCustomWithConnectionLimit(dbName, username, password string, connectionLimit int) (ProvisioningOwnership, error) {
	return s.ProvisionDatabaseCustomWithConnectionLimitCheckpoint(dbName, username, password, connectionLimit, nil)
}

// ProvisionDatabaseCustomWithConnectionLimitCheckpoint persists ownership immediately after each DDL step.
func (s *MySQLService) ProvisionDatabaseCustomWithConnectionLimitCheckpoint(dbName, username, password string, connectionLimit int, checkpoint func(ProvisioningOwnership) error) (ProvisioningOwnership, error) {
	var ownership ProvisioningOwnership
	dbName = strings.ToLower(dbName)
	username = strings.ToLower(username)

	var nameRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)
	var userRegex = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
	var passRegex = regexp.MustCompile(`^[^[:space:]"'\x60\\;@#/?]{12,128}$`)

	if !nameRegex.MatchString(dbName) {
		return ownership, apperr.New(400, "INVALID_DB_NAME", "Database name must be 2-64 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if !userRegex.MatchString(username) {
		return ownership, apperr.New(400, "INVALID_DB_USER", "Database username must be 2-32 characters, start with a letter, and contain only alphanumeric characters or underscores")
	}
	if !passRegex.MatchString(password) {
		return ownership, apperr.New(400, "INVALID_DB_PASSWORD", "Database password must be 12-128 characters and not contain invalid characters")
	}
	if connectionLimit <= 0 {
		return ownership, apperr.New(400, "INVALID_DB_CONNECTION_LIMIT", "Database connection limit must be positive")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	// NOTE: Check-then-create has an inherent TOCTOU window because MySQL/PostgreSQL
	// DDL is non-transactional. This is acceptable because global control-plane
	// uniqueness is enforced before this call reaches the infrastructure layer.

	// Check if database already exists
	checkDBSQL := fmt.Sprintf("SELECT SCHEMA_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = '%s';", dbName)
	checkDBRes, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword, "-N", "-e", checkDBSQL)
	if err != nil {
		return ownership, apperr.New(503, "ENGINE_UNREACHABLE", "Could not verify existence of database/role; provisioning aborted")
	}
	if strings.TrimSpace(checkDBRes.Stdout) == dbName {
		return ownership, apperr.New(409, "DB_ALREADY_EXISTS", fmt.Sprintf("MySQL database '%s' already exists", dbName))
	}

	// Check if user already exists
	checkUserSQL := fmt.Sprintf("SELECT User FROM mysql.user WHERE User = '%s';", username)
	checkUserRes, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword, "-N", "-e", checkUserSQL)
	if err != nil {
		return ownership, apperr.New(503, "ENGINE_UNREACHABLE", "Could not verify existence of database/role; provisioning aborted")
	}
	if strings.TrimSpace(checkUserRes.Stdout) == username {
		return ownership, apperr.New(409, "USER_ALREADY_EXISTS", fmt.Sprintf("MySQL user '%s' already exists", username))
	}

	res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("CREATE DATABASE `%s`;", dbName))
	if err != nil {
		return ownership, apperr.New(500, "DB_CREATE_FAILED", "Failed to create MySQL database: "+res.Stderr)
	}
	ownership.DatabaseCreated = true
	if checkpoint != nil {
		if err := checkpoint(ownership); err != nil {
			return ownership, err
		}
	}

	res, err = utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("CREATE USER '%s'@'%%' IDENTIFIED BY '%s';", username, password))
	if err != nil {
		return ownership, apperr.New(500, "DB_PROVISION_FAILED", "Failed to create database user: "+res.Stderr)
	}
	ownership.UserCreated = true
	if checkpoint != nil {
		if err := checkpoint(ownership); err != nil {
			return ownership, err
		}
	}

	res, err = utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("ALTER USER '%s'@'%%' WITH MAX_USER_CONNECTIONS %d; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;", username, connectionLimit, dbName, username))
	if err != nil {
		return ownership, apperr.New(500, "DB_PROVISION_FAILED", "Failed to configure database user: "+res.Stderr)
	}

	return ownership, nil
}

// DropDatabaseCustom removes a MySQL database and its associated user
func (s *MySQLService) DropDatabaseCustom(dbName, username string) error {
	return s.DropDatabaseCustomOwned(dbName, username, ProvisioningOwnership{DatabaseCreated: true, UserCreated: true})
}

// DropDatabaseCustomOwned removes only resources proven to be provisioned here.
func (s *MySQLService) DropDatabaseCustomOwned(dbName, username string, ownership ProvisioningOwnership) error {
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

	if !ownership.HasResources() {
		return nil
	}
	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")
	statements := make([]string, 0, 2)
	if ownership.DatabaseCreated {
		statements = append(statements, fmt.Sprintf("DROP DATABASE IF EXISTS `%s`;", dbName))
	}
	if ownership.UserCreated {
		statements = append(statements, fmt.Sprintf("DROP USER IF EXISTS '%s'@'%%';", username))
	}
	res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", strings.Join(statements, " "))
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

	res, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", dbName))
	if err != nil {
		return apperr.New(500, "DB_CREATE_FAILED", "Failed to create MySQL database: "+res.Stderr)
	}

	// Create user with connection limit to prevent tenant connection storms and SQL injection
	res, err = utils.Run(1*time.Minute, "docker", "exec", s.containerName,
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
	_, err := utils.Run(1*time.Minute, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'%%';", dbName, dbName))
	return err
}

// UpdatePassword rotates the database user's password inside the MySQL engine.
func (s *MySQLService) UpdatePassword(username, newPassword string) error {
	if !validDBNameRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validPasswordRegex.MatchString(newPassword) {
		return apperr.New(400, "INVALID_DB_PASSWORD", "Database password must contain only alphanumeric characters and be 8-128 characters long")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")
	res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("ALTER USER '%s'@'%%' IDENTIFIED BY '%s'; FLUSH PRIVILEGES;", username, newPassword))
	if err != nil {
		return apperr.New(500, "DB_PASSWORD_FAILED", "Failed to update MySQL password: "+res.Stderr)
	}
	return nil
}

// UpdateStatus uses account lock/unlock so retries converge after a metadata-write failure.
func (s *MySQLService) UpdateStatus(dbName, username string, connectionLimit int, suspend bool) error {
	if !validDBNameRegex.MatchString(dbName) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if !validDBNameRegex.MatchString(username) {
		return apperr.New(400, "INVALID_DB_NAME", "Database name must match ^[a-z0-9_]{3,63}$")
	}
	if connectionLimit <= 0 {
		return apperr.New(400, "INVALID_CONNECTION_LIMIT", "Database connection limit must be positive")
	}

	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	if suspend {
		res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"mysql", "-uroot", "-p"+rootPassword,
			"-e", fmt.Sprintf("ALTER USER '%s'@'%%' ACCOUNT LOCK; FLUSH PRIVILEGES;", username))
		if err != nil {
			return apperr.New(500, "DB_SUSPEND_FAILED", "Failed to suspend MySQL database: "+res.Stderr)
		}
	} else {
		res, err := utils.Run(30*time.Second, "docker", "exec", s.containerName,
			"mysql", "-uroot", "-p"+rootPassword,
			"-e", fmt.Sprintf("ALTER USER '%s'@'%%' ACCOUNT UNLOCK; ALTER USER '%s'@'%%' WITH MAX_USER_CONNECTIONS %d; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;", username, username, connectionLimit, dbName, username))
		if err != nil {
			return apperr.New(500, "DB_RESUME_FAILED", "Failed to resume MySQL database: "+res.Stderr)
		}
	}

	return nil
}
