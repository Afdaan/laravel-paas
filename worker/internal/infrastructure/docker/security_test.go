package docker

import (
	"strings"
	"testing"
)

func TestTenantHardeningArgs(t *testing.T) {
	args := TenantHardeningArgs("512m")

	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}

	str := strings.Join(args, " ")
	if !strings.Contains(str, "--memory-swap 512m") {
		t.Errorf("missing --memory-swap: %s", str)
	}
	if !strings.Contains(str, "--security-opt=no-new-privileges:true") {
		t.Errorf("missing no-new-privileges: %s", str)
	}
	if !strings.Contains(str, "--pids-limit=250") {
		t.Errorf("missing pids-limit: %s", str)
	}
}

func TestTenantHardeningArgsEmptyMemory(t *testing.T) {
	args := TenantHardeningArgs("")
	if len(args) != 4 {
		t.Fatalf("expected 4 args even with empty memory, got %d", len(args))
	}
}