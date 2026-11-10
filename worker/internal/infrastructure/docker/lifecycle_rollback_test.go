package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	sharedDocker "github.com/laravel-paas/shared/infrastructure/docker"
	"github.com/laravel-paas/shared/models"
)

func TestRollbackMissingTargetNeverStartsLatest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	service := &DockerService{}
	project := &models.Project{Subdomain: "lost-image", LastCommitHash: strings.Repeat("a", 40)}
	if _, err := service.StartExistingImage(project, "example.test", true); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing target image must fail before latest fallback: %v", err)
	}
}

func TestRollbackImageRemovedBetweenInspectAndRun(t *testing.T) {
	dir := t.TempDir()
	runLog := filepath.Join(dir, "run-args")
	command := "#!/bin/sh\ncase \"$1\" in\n image) echo '{}'; exit 0;;\n run) echo \"$*\" > \"$DOCKER_TEST_RUN_LOG\"; exit 1;;\nesac\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(command), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_RUN_LOG", runLog)
	cfg := &config.Config{DataPath: filepath.Join(dir, "data"), HostDataPath: filepath.Join(dir, "data"), ProjectsPath: filepath.Join(dir, "projects")}
	storage := infrastructure.NewStorageService(cfg, nil)
	service := &DockerService{DockerService: sharedDocker.NewDockerService(cfg, storage, nil), cfg: cfg, storage: storage}
	port := 8080
	project := &models.Project{UserID: 1, Subdomain: "lost-image", LastCommitHash: strings.Repeat("a", 40), Port: &port}
	if _, err := service.StartExistingImage(project, "example.test", true); err == nil {
		t.Fatal("missing image must not start another tag")
	}
	args, err := os.ReadFile(runLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--pull=never") || !strings.HasSuffix(strings.TrimSpace(string(args)), "paas-lost-image:"+project.LastCommitHash) {
		t.Fatalf("rollback image could be pulled or replaced: %s", args)
	}
}

func TestRestartMissingCommitTagUsesServingImage(t *testing.T) {
	dir := t.TempDir()
	runLog := filepath.Join(dir, "run-args")
	imageID := "sha256:" + strings.Repeat("b", 64)
	command := "#!/bin/sh\ncase \"$1\" in\n inspect) echo \"$DOCKER_TEST_IMAGE_ID\";;\n image) exit 1;;\n run) echo \"$*\" > \"$DOCKER_TEST_RUN_LOG\"; echo new-container;;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(command), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("DOCKER_TEST_IMAGE_ID", imageID)
	t.Setenv("DOCKER_TEST_RUN_LOG", runLog)
	cfg := &config.Config{DataPath: filepath.Join(dir, "data"), HostDataPath: filepath.Join(dir, "data"), ProjectsPath: filepath.Join(dir, "projects")}
	storage := infrastructure.NewStorageService(cfg, nil)
	service := &DockerService{DockerService: sharedDocker.NewDockerService(cfg, storage, nil), cfg: cfg, storage: storage}
	port := 8080
	containerID := "serving-container"
	project := &models.Project{UserID: 1, Subdomain: "running-app", ContainerID: &containerID, LastCommitHash: strings.Repeat("a", 40), Port: &port}
	if _, err := service.StartExistingImage(project, "example.test", true); err == nil {
		t.Fatal("rollback must reject missing commit tag")
	}
	if _, err := os.Stat(runLog); !os.IsNotExist(err) {
		t.Fatalf("rollback attempted image start: %v", err)
	}
	newID, err := service.StartExistingImage(project, "example.test", false)
	if err != nil || newID != "new-container" {
		t.Fatalf("restart with serving image: id=%q err=%v", newID, err)
	}
	args, err := os.ReadFile(runLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "--pull=never") || !strings.HasSuffix(strings.TrimSpace(string(args)), imageID) {
		t.Fatalf("restart did not pin serving image: %s", args)
	}
}
