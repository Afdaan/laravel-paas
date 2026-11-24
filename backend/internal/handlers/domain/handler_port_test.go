package domain

import (
	"testing"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
)

func TestEnsureDomainRoutingEnabled(t *testing.T) {
	workerPort := 0
	err := ensureDomainRoutingEnabled(&models.Project{Port: &workerPort})
	appErr, ok := err.(*apperr.AppError)
	if !ok || appErr.HTTPStatus != 409 || appErr.Code != "PROJECT_NOT_WEB_FACING" {
		t.Fatalf("worker-only error=%#v", err)
	}

	invalidPort := -1
	err = ensureDomainRoutingEnabled(&models.Project{Port: &invalidPort})
	appErr, ok = err.(*apperr.AppError)
	if !ok || appErr.HTTPStatus != 400 || appErr.Code != "INVALID_PORT" {
		t.Fatalf("invalid-port error=%#v", err)
	}

	webPort := 8080
	if err := ensureDomainRoutingEnabled(&models.Project{Port: &webPort}); err != nil {
		t.Fatalf("web project error=%v", err)
	}

	if err := ensureDomainRoutingEnabled(&models.Project{}); err != nil {
		t.Fatalf("auto-port project error=%v", err)
	}
}
