package worker

import (
	"strings"
	"testing"

	"github.com/laravel-paas/shared/config"
)

func TestBuildWorkerRunArgsIncludesProvisioningEnv(t *testing.T) {
	t.Setenv("MYSQL_CONTAINER_NAME", "custom-mysql-svc")
	t.Setenv("POSTGRES_CONTAINER_NAME", "custom-pg-svc")

	cfg := &config.Config{
		HostProjectsPath:  "/srv/paas/projects",
		HostDataPath:      "/srv/paas/data",
		HostTemplatesPath: "/srv/paas/templates",
		HostRailpacksPath: "/srv/paas/railpacks",
		DockerSocket:      "/var/run/docker.sock",
		PGHost:            "paas-postgres",
		PGPort:            "5432",
		PGUser:            "paas",
		PGPassword:        "pgsecret",
		PGDatabase:        "paas",
		RedisHost:         "paas-redis",
		RedisPort:         "6379",
		RedisPassword:     "redissecret",
		MYSQLHost:         "custom-mysql-svc",
		MYSQLPort:         "3306",
		MYSQLUser:         "root",
		MYSQLPassword:     "rootsecret",
		MYSQLDatabase:     "paas",
		MYSQLRootPassword: "custom-root-password-123",
		UserPGHost:        "custom-pg-svc",
		UserPGPort:        "5432",
		UserPGPassword:    "user-pg-secret-456",
		BaseDomain:        "example.com",
		ProjectDomain:     "example.com",
		AppEnv:            "production",
	}

	args := buildWorkerRunArgs(cfg, 2, "v1.2.3", "paas-worker-s2-v1-2-3")

	argMap := make(map[string]string)
	for i := 0; i < len(args); i++ {
		if args[i] == "-e" && i+1 < len(args) {
			pair := args[i+1]
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				argMap[parts[0]] = parts[1]
			}
		}
	}

	requiredChecks := map[string]string{
		"MYSQL_ROOT_PASSWORD":     "custom-root-password-123",
		"MYSQL_CONTAINER_NAME":    "custom-mysql-svc",
		"POSTGRES_CONTAINER_NAME": "custom-pg-svc",
		"USER_PG_HOST":            "custom-pg-svc",
		"USER_PG_PORT":            "5432",
		"USER_PG_PASSWORD":        "user-pg-secret-456",
		"SLOT":                    "2",
		"VERSION":                 "v1.2.3",
	}

	for k, expected := range requiredChecks {
		actual, exists := argMap[k]
		if !exists {
			t.Errorf("expected env %s to be set in worker runArgs", k)
		} else if actual != expected {
			t.Errorf("expected env %s=%q, got %q", k, expected, actual)
		}
	}
}
