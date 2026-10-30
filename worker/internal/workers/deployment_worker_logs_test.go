package workers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestActiveLogPathUsesCanonicalJobDirectory(t *testing.T) {
	worker := &DeploymentWorker{cfg: &config.Config{ProjectsPath: t.TempDir()}}
	jobID := "job-current"
	project := &models.Project{ID: 42, UserID: 7, UserSlug: "name-7", Subdomain: "app", DeploymentJobID: &jobID}
	buildPath, err := project.GetDeploymentLogPath(worker.cfg.ProjectsPath, jobID, "build")
	if err != nil {
		t.Fatal(err)
	}
	if got := worker.getActiveLogPath(project); got != buildPath {
		t.Fatalf("new job log = %q, want %q", got, buildPath)
	}
	infraPath, err := project.GetDeploymentLogPath(worker.cfg.ProjectsPath, jobID, "infra")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(infraPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(infraPath, []byte("restart output"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := worker.getActiveLogPath(project); got != infraPath {
		t.Fatalf("infrastructure job log = %q, want %q", got, infraPath)
	}
	if err := os.WriteFile(buildPath, []byte("build output"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := worker.getActiveLogPath(project); got != buildPath {
		t.Fatalf("build job log = %q, want %q", got, buildPath)
	}
	invalidJobID := "../../foreign"
	project.DeploymentJobID = &invalidJobID
	if got := worker.getActiveLogPath(project); got != "" {
		t.Fatalf("invalid job ID accepted: %q", got)
	}
}

func TestDeploymentStartMessageHidesInternalJobType(t *testing.T) {
	tests := map[string]string{
		"deploy":         "Starting initial deployment",
		"redeploy":       "Starting redeployment",
		"redeploy_clean": "Starting clean rebuild",
		"rollback":       "Starting rollback",
		"unknown":        "Starting deployment",
	}

	for jobType, expected := range tests {
		if actual := deploymentStartMessage(jobType); actual != expected {
			t.Errorf("deploymentStartMessage(%q) = %q, want %q", jobType, actual, expected)
		}
	}
}
