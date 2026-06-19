package services

import (
	"strings"
	"testing"

	"github.com/laravel-paas/shared/models"
)

func TestBuildDSN(t *testing.T) {
	uidSalt := "test-salt"

	// 1. MySQL test where Name != Username
	mysqlInstance := &models.DatabaseInstance{
		Engine:   "mysql",
		Host:     "127.0.0.1",
		Port:     3306,
		Name:     "custom_db_name",
		Username: "custom_db_user",
		Password: "secret_password",
	}

	mysqlDSN := buildDSN(mysqlInstance, uidSalt)
	expectedMySQL := "custom_db_user:secret_password@tcp(127.0.0.1:3306)/custom_db_name?parseTime=true"
	if mysqlDSN != expectedMySQL {
		t.Errorf("Expected MySQL DSN %q, got %q", expectedMySQL, mysqlDSN)
	}

	// 2. PostgreSQL test where Name != Username
	postgresInstance := &models.DatabaseInstance{
		Engine:   "postgresql",
		Host:     "paas-user-postgres",
		Port:     5432,
		Name:     "pg_db_name",
		Username: "pg_db_user",
		Password: "pg_password",
	}

	postgresDSN := buildDSN(postgresInstance, uidSalt)
	if !strings.Contains(postgresDSN, "postgres://pg_db_user:pg_password@paas-user-postgres:5432/pg_db_name") {
		t.Errorf("Expected PostgreSQL DSN to contain correct credentials, host, and port, got %q", postgresDSN)
	}
	if !strings.Contains(postgresDSN, "application_name=paas-backend-") {
		t.Errorf("Expected PostgreSQL DSN to contain salted application name, got %q", postgresDSN)
	}
}
