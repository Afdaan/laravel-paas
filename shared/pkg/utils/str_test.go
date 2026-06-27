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
