package docker

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/laravel-paas/shared/apperr"
)

func TestGetBuildPathPrefersBackendMarkerOverRootPackage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{}`)
	writeTestFile(t, filepath.Join(root, "backend", "artisan"), ``)
	writeTestFile(t, filepath.Join(root, "backend", "composer.json"), `{"require":{"laravel/framework":"^12.0"}}`)

	service := &DockerService{}
	if got := service.GetBuildPath(root); got != filepath.Join(root, "backend") {
		t.Fatalf("GetBuildPath() = %s, want %s", got, filepath.Join(root, "backend"))
	}
}

func TestGetBuildPathPrefersStrongMarkerWithoutDirectoryNameHint(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "package.json"), `{}`)
	writeTestFile(t, filepath.Join(root, "service", "go.mod"), "module example.com/service\n\ngo 1.24\n")

	service := &DockerService{}
	if got := service.GetBuildPath(root); got != filepath.Join(root, "service") {
		t.Fatalf("GetBuildPath() = %s, want %s", got, filepath.Join(root, "service"))
	}
}

func TestGetBuildPathKeepsStrongRootOverNestedApp(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "artisan"), ``)
	writeTestFile(t, filepath.Join(root, "composer.json"), `{"require":{"laravel/framework":"^12.0"}}`)
	writeTestFile(t, filepath.Join(root, "backend", "go.mod"), "module example.com/backend\n\ngo 1.24\n")

	service := &DockerService{}
	if got := service.GetBuildPath(root); got != root {
		t.Fatalf("GetBuildPath() = %s, want %s", got, root)
	}
}

func TestGetBuildPathUsesMultipleBackendMarkers(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "composer.json"), `{}`)
	writeTestFile(t, filepath.Join(root, "service", "artisan"), ``)
	writeTestFile(t, filepath.Join(root, "service", "composer.json"), `{"require":{"laravel/framework":"^12.0"}}`)

	service := &DockerService{}
	if got := service.GetBuildPath(root); got != filepath.Join(root, "service") {
		t.Fatalf("GetBuildPath() = %s, want %s", got, filepath.Join(root, "service"))
	}
}

func TestResolveBuildPathRejectsInvalidExplicitDirectory(t *testing.T) {
	service := &DockerService{}
	_, err := service.ResolveBuildPath(t.TempDir(), "missing")
	if err == nil {
		t.Fatal("ResolveBuildPath() error = nil, want INVALID_BASE_DIRECTORY")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "INVALID_BASE_DIRECTORY" {
		t.Fatalf("ResolveBuildPath() error = %v, want INVALID_BASE_DIRECTORY", err)
	}
}

func TestResolveBuildPathRejectsWindowsSeparator(t *testing.T) {
	service := &DockerService{}
	_, err := service.ResolveBuildPath(t.TempDir(), `backend\app`)
	if err == nil {
		t.Fatal("ResolveBuildPath() error = nil, want INVALID_BASE_DIRECTORY")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
