// ===========================================
// MySQL Provisioning Service
// ===========================================
// Handles MySQL database lifecycle for
// student projects (create, drop)
// ===========================================
package services

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"github.com/laravel-paas/backend/internal/apperr"
)

// MySQLService handles MySQL database provisioning for student projects
type MySQLService struct{}

// NewMySQLService creates a new MySQL provisioning service
func NewMySQLService() *MySQLService {
	return &MySQLService{}
}

// CreateDatabase creates a MySQL database and matching user for a student project
func (s *MySQLService) CreateDatabase(dbName string) error {
	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	// Step 1: Create the database
	cmd := exec.Command("docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", dbName))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return apperr.New(500, "DB_CREATE_FAILED", "Failed to create MySQL database: "+stderr.String())
	}

	// Step 2: Create user with matching credentials and grant full access
	cmd = exec.Command("docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf(
			"CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;",
			dbName, dbName, dbName, dbName,
		))

	stderr.Reset()
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return apperr.New(500, "DB_PROVISION_FAILED", "Failed to create database user or grant permissions: "+stderr.String())
	}

	return nil
}

// DropDatabase removes a MySQL database and its associated user
func (s *MySQLService) DropDatabase(dbName string) error {
	rootPassword := os.Getenv("MYSQL_ROOT_PASSWORD")

	cmd := exec.Command("docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+rootPassword,
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'%%';", dbName, dbName))

	// Best-effort cleanup: errors are non-fatal during deletion
	cmd.Run()
	return nil
}
