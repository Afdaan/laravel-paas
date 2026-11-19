package project

import (
	"errors"
	"fmt"
	"testing"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/models"
)

func TestBuildProjectProxyTargetRejectsNonWebProjects(t *testing.T) {
	containerID := "container-id"
	for _, port := range []int{-1, 0, models.MaxRuntimePort + 1} {
		t.Run(fmt.Sprintf("port_%d", port), func(t *testing.T) {
			project := &models.Project{ContainerID: &containerID, Port: &port}
			_, err := buildProjectProxyTarget(project, "/")
			var appError *apperr.AppError
			if !errors.As(err, &appError) || appError.HTTPStatus != 503 || appError.Code != "PROJECT_NOT_WEB_FACING" {
				t.Fatalf("port %d error=%v", port, err)
			}
		})
	}
}

func TestBuildProjectProxyTargetPreservesRequestPath(t *testing.T) {
	containerID := "container-id"
	port := 8080
	project := &models.Project{ContainerID: &containerID, Port: &port}

	target, err := buildProjectProxyTarget(project, "api/health?full=1")
	if err != nil {
		t.Fatal(err)
	}
	if target != "http://container-id:8080/api/health?full=1" {
		t.Fatalf("target=%q", target)
	}
}
