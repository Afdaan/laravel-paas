package utils

import (
	"os"
	"strings"
	"testing"
)

func TestRedactInfrastructureDetails(t *testing.T) {
	// Test default redactions
	input := "Error connecting to paas-mysql and paas-user-postgres at /home/afdaan/app"
	redacted := RedactInfrastructureDetails(input, nil)

	if strings.Contains(redacted, "paas-mysql") {
		t.Error("Expected 'paas-mysql' to be redacted")
	}
	if strings.Contains(redacted, "paas-user-postgres") {
		t.Error("Expected 'paas-user-postgres' to be redacted")
	}

	// Test custom env redactions
	os.Setenv("MYSQL_CONTAINER_NAME", "custom-mysql-container")
	os.Setenv("POSTGRES_CONTAINER_NAME", "custom-postgres-container")
	defer func() {
		os.Unsetenv("MYSQL_CONTAINER_NAME")
		os.Unsetenv("POSTGRES_CONTAINER_NAME")
	}()

	inputCustom := "Error on custom-mysql-container and custom-postgres-container"
	redactedCustom := RedactInfrastructureDetails(inputCustom, nil)

	if strings.Contains(redactedCustom, "custom-mysql-container") {
		t.Error("Expected custom MySQL container name to be redacted")
	}
	if strings.Contains(redactedCustom, "custom-postgres-container") {
		t.Error("Expected custom Postgres container name to be redacted")
	}
}

func TestMigrationSchemaConflict(t *testing.T) {
	output := `2026_07_12_200000_create_all_tables_consolidated_v2 ........... 57.84ms FAIL
SQLSTATE[42S01]: Base table or view already exists: 1050 Table 'users' already exists`

	if code := MigrationErrorCode(output); code != "MIGRATION_SCHEMA_CONFLICT" {
		t.Fatalf("MigrationErrorCode() = %q, want MIGRATION_SCHEMA_CONFLICT", code)
	}

	summary := MigrationFailureSummary(output)
	for _, expected := range []string{"`users`", "`2026_07_12_200000_create_all_tables_consolidated_v2`", "not automatically rolled back"} {
		if !strings.Contains(summary, expected) {
			t.Errorf("MigrationFailureSummary() missing %q: %s", expected, summary)
		}
	}

	suggestion := GetSmartSuggestion(output)
	if !strings.Contains(suggestion, "must not rerun a consolidated create-all migration") {
		t.Errorf("GetSmartSuggestion() returned unexpected guidance: %s", suggestion)
	}
}

func TestGenericMigrationFailureWarnsAboutPartialChanges(t *testing.T) {
	summary := SanitizeError("[MIGRATION_FAILED] Migrations failed: command exited with status 1")
	if !strings.Contains(summary, "not automatically rolled back") {
		t.Fatalf("SanitizeError() must warn about partial database changes: %s", summary)
	}
}
