package docker

import (
	"reflect"
	"testing"

	"github.com/laravel-paas/shared/models"
)

func TestRuntimePortEnvArgs(t *testing.T) {
	web := models.RuntimeExposure{WebFacing: true, Port: 8080}
	if got := runtimePortEnvArgs(web); !reflect.DeepEqual(got, []string{"-e", "PORT=8080"}) {
		t.Fatalf("web env args=%v", got)
	}
	if got, ok := runtimePortEnvValue(web); !ok || got != "8080" {
		t.Fatalf("web env value=%q ok=%v", got, ok)
	}

	workerOnly := models.RuntimeExposure{WebFacing: false, Reason: models.RuntimeExposureReasonDisabledPort}
	if got := runtimePortEnvArgs(workerOnly); len(got) != 0 {
		t.Fatalf("worker-only env args=%v", got)
	}
	if got, ok := runtimePortEnvValue(workerOnly); ok || got != "" {
		t.Fatalf("worker-only env value=%q ok=%v", got, ok)
	}

	invalid := models.RuntimeExposure{WebFacing: true, Port: 0}
	if got, ok := runtimePortEnvValue(invalid); ok || got != "" {
		t.Fatalf("invalid env value=%q ok=%v", got, ok)
	}
}
