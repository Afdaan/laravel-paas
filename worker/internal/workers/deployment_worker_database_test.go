package workers

import (
	"context"
	"errors"
	"strings"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/laravel-paas/shared/pkg/utils"
)

func TestEnsureManagedDatabase(t *testing.T) {
	t.Run("skips provisioning when database is already reachable", func(t *testing.T) {
		provisionCalls := 0

		version, err := ensureManagedDatabase(
			func() (string, error) { return "MySQL 8.0.36", nil },
			func() error {
				provisionCalls++
				return nil
			},
		)

		if err != nil {
			t.Fatalf("ensureManagedDatabase() error = %v", err)
		}
		if version != "MySQL 8.0.36" {
			t.Fatalf("ensureManagedDatabase() version = %q", version)
		}
		if provisionCalls != 0 {
			t.Fatalf("provision called %d times", provisionCalls)
		}
	})

	t.Run("provisions missing database then verifies it", func(t *testing.T) {
		verifyCalls := 0

		version, err := ensureManagedDatabase(
			func() (string, error) {
				verifyCalls++
				if verifyCalls == 1 {
					return "", errors.New("database does not exist")
				}
				return "PostgreSQL 16.3", nil
			},
			func() error { return nil },
		)

		if err != nil {
			t.Fatalf("ensureManagedDatabase() error = %v", err)
		}
		if version != "PostgreSQL 16.3" {
			t.Fatalf("ensureManagedDatabase() version = %q", version)
		}
		if verifyCalls != 2 {
			t.Fatalf("verify called %d times", verifyCalls)
		}
	})

	t.Run("accepts concurrent already-exists result when verification succeeds", func(t *testing.T) {
		verifyCalls := 0

		_, err := ensureManagedDatabase(
			func() (string, error) {
				verifyCalls++
				if verifyCalls == 1 {
					return "", errors.New("database does not exist")
				}
				return "MySQL 8.0.36", nil
			},
			func() error { return errors.New("[DB_ALREADY_EXISTS] database already exists") },
		)

		if err != nil {
			t.Fatalf("ensureManagedDatabase() error = %v", err)
		}
	})

	t.Run("preserves provisioning error when database remains unreachable", func(t *testing.T) {
		provisionErr := errors.New("engine unreachable")

		_, err := ensureManagedDatabase(
			func() (string, error) { return "", errors.New("connection refused") },
			func() error { return provisionErr },
		)

		if !errors.Is(err, provisionErr) {
			t.Fatalf("ensureManagedDatabase() error = %v", err)
		}
	})
}

func TestInspectManagedDatabaseRejectsUnsupportedEngine(t *testing.T) {
	_, err := inspectManagedDatabase(context.Background(), "sqlite", "localhost", 0, "app", "app", "password")
	if err == nil || !strings.Contains(err.Error(), "unsupported managed database engine") {
		t.Fatalf("inspectManagedDatabase() error = %v", err)
	}
}

func TestBuildManagedDatabaseConnectionPreservesValidatedPasswords(t *testing.T) {
	passwords := []string{
		"ValidPass%123",
		"ValidPass:123",
		"ValidPass+123",
		"ValidPass=123",
	}

	for _, password := range passwords {
		if err := utils.ValidateDatabasePassword(password); err != nil {
			t.Fatalf("test password %q rejected by validator: %v", password, err)
		}

		t.Run("mysql_"+password, func(t *testing.T) {
			connection, err := buildManagedDatabaseConnection("mysql", "database.internal", 3306, "app_db", "app_user", password)
			if err != nil {
				t.Fatalf("buildManagedDatabaseConnection() error = %v", err)
			}

			config, err := mysqlDriver.ParseDSN(connection.dsn)
			if err != nil {
				t.Fatalf("mysql.ParseDSN() error = %v", err)
			}
			if config.Passwd != password {
				t.Fatalf("parsed MySQL password does not match original")
			}
		})

		t.Run("postgresql_"+password, func(t *testing.T) {
			connection, err := buildManagedDatabaseConnection("postgresql", "database.internal", 5432, "app_db", "app_user", password)
			if err != nil {
				t.Fatalf("buildManagedDatabaseConnection() error = %v", err)
			}

			config, err := pgx.ParseConfig(connection.dsn)
			if err != nil {
				t.Fatalf("pgx.ParseConfig() error = %v", err)
			}
			if config.Password != password {
				t.Fatalf("parsed PostgreSQL password does not match original")
			}
		})
	}
}
