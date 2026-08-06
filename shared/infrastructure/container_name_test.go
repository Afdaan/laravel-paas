package infrastructure

import (
	"strings"
	"testing"
)

func TestMySQLContainerName(t *testing.T) {
	// Test default fallback
	t.Setenv("MYSQL_CONTAINER_NAME", "")
	if name := MySQLContainerName(); name != "paas-mysql" {
		t.Errorf("Expected default 'paas-mysql', got '%s'", name)
	}

	// Test override
	t.Setenv("MYSQL_CONTAINER_NAME", "custom-mysql")
	if name := MySQLContainerName(); name != "custom-mysql" {
		t.Errorf("Expected override 'custom-mysql', got '%s'", name)
	}
}

func TestPostgreSQLContainerName(t *testing.T) {
	// Test default fallback
	t.Setenv("POSTGRES_CONTAINER_NAME", "")
	if name := PostgreSQLContainerName(); name != "paas-user-postgres" {
		t.Errorf("Expected default 'paas-user-postgres', got '%s'", name)
	}

	// Test override
	t.Setenv("POSTGRES_CONTAINER_NAME", "custom-postgres")
	if name := PostgreSQLContainerName(); name != "custom-postgres" {
		t.Errorf("Expected override 'custom-postgres', got '%s'", name)
	}
}

func TestMySQLPort(t *testing.T) {
	t.Setenv("MYSQL_PORT", "")
	if port := MySQLPort(); port != 3306 {
		t.Errorf("Expected default port 3306, got %d", port)
	}

	t.Setenv("MYSQL_PORT", "3307")
	if port := MySQLPort(); port != 3307 {
		t.Errorf("Expected override port 3307, got %d", port)
	}
}

func TestPostgreSQLPort(t *testing.T) {
	// Mode: docker
	t.Setenv("APP_MODE", "docker")
	t.Setenv("USER_PG_PORT", "")
	if port := PostgreSQLPort(); port != 5432 {
		t.Errorf("Expected internal port 5432 in docker mode, got %d", port)
	}

	t.Setenv("USER_PG_PORT", "5544")
	if port := PostgreSQLPort(); port != 5432 {
		t.Errorf("Expected internal port 5432 in docker mode regardless of override, got %d", port)
	}

	// Mode: other (local/prod)
	t.Setenv("APP_MODE", "local")
	t.Setenv("USER_PG_PORT", "")
	if port := PostgreSQLPort(); port != 5433 {
		t.Errorf("Expected fallback port 5433 when not in docker mode, got %d", port)
	}

	t.Setenv("USER_PG_PORT", "5544")
	if port := PostgreSQLPort(); port != 5544 {
		t.Errorf("Expected override port 5544, got %d", port)
	}
}

func TestDatabasePortFallsBackForInvalidValues(t *testing.T) {
	for _, value := range []string{"abc", "0", "65536", "-1"} {
		t.Setenv("MYSQL_PORT", value)
		if port := MySQLPort(); port != 3306 {
			t.Errorf("Expected fallback port 3306 for %q, got %d", value, port)
		}
	}
}

func TestMySQLUpdateStatusValidation(t *testing.T) {
	s := NewMySQLService()

	// 1. Invalid DB name
	err := s.UpdateStatus("invalid-name-!", "valid_user", DefaultManagedDatabaseConnectionLimit, true)
	if err == nil || !strings.Contains(err.Error(), "INVALID_DB_NAME") {
		t.Errorf("Expected INVALID_DB_NAME error, got %v", err)
	}

	// 2. Invalid Username
	err = s.UpdateStatus("valid_db", "invalid-user-!", DefaultManagedDatabaseConnectionLimit, true)
	if err == nil || !strings.Contains(err.Error(), "INVALID_DB_NAME") {
		t.Errorf("Expected INVALID_DB_NAME error for username, got %v", err)
	}
}

func TestPostgreSQLUpdateStatusValidation(t *testing.T) {
	s := NewPostgreSQLService()

	// 1. Invalid DB name
	err := s.UpdateStatus("invalid-name-!", "valid_user", true)
	if err == nil || !strings.Contains(err.Error(), "INVALID_DB_NAME") {
		t.Errorf("Expected INVALID_DB_NAME error, got %v", err)
	}

	// 2. Invalid Username
	err = s.UpdateStatus("valid_db", "invalid-user-!", true)
	if err == nil || !strings.Contains(err.Error(), "INVALID_DB_NAME") {
		t.Errorf("Expected INVALID_DB_NAME error for username, got %v", err)
	}
}

func TestPostgreSQLSuspendSQLRevokesPublicAndVerifiesSessions(t *testing.T) {
	revokeSQL, terminateSQL, remainingSQL := postgreSQLSuspendSQL("tenant_db", "tenant_user")
	if !strings.Contains(revokeSQL, "NOLOGIN") || !strings.Contains(revokeSQL, "FROM PUBLIC") || !strings.Contains(revokeSQL, "FROM \"tenant_user\"") {
		t.Fatalf("revoke sql=%q", revokeSQL)
	}
	if !strings.Contains(terminateSQL, "pg_terminate_backend") || !strings.Contains(terminateSQL, "datname = 'tenant_db'") {
		t.Fatalf("terminate sql=%q", terminateSQL)
	}
	if !strings.Contains(remainingSQL, "count(*)") || !strings.Contains(remainingSQL, "datname = 'tenant_db'") {
		t.Fatalf("remaining sql=%q", remainingSQL)
	}
	if resumeSQL := postgreSQLResumeSQL("tenant_db", "tenant_user"); !strings.Contains(resumeSQL, "LOGIN") || !strings.Contains(resumeSQL, "GRANT CONNECT") {
		t.Fatalf("resume sql=%q", resumeSQL)
	}
}
