package models

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDeploymentLogPathUsesStableTenantProjectAndJob(t *testing.T) {
	project := Project{ID: 42, UserID: 7, UserSlug: "name-7", Subdomain: "app"}
	root := t.TempDir()
	want := filepath.Join(root, ".deployment-logs", "user-7", "project-42", "build-job-1.log")
	got, err := project.GetDeploymentLogPath(root, "job-1", "build")
	if err != nil || got != want {
		t.Fatalf("log path = %q, %v; want %q", got, err, want)
	}
	project.UserSlug = "renamed-7"
	project.Subdomain = "renamed-app"
	if changed, err := project.GetDeploymentLogPath(root, "job-1", "build"); err != nil || changed != got {
		t.Fatalf("renaming changed log identity: %q, %v", changed, err)
	}
	project.UserID++
	if other, err := project.GetDeploymentLogPath(root, "job-1", "build"); err != nil || other == got {
		t.Fatalf("another tenant shares log path: %q, %v", other, err)
	}
	project.UserID--
	project.ID++
	if other, err := project.GetDeploymentLogPath(root, "job-1", "build"); err != nil || other == got {
		t.Fatalf("another project shares log path: %q, %v", other, err)
	}
}

func TestDeploymentLogPathRejectsInvalidJobIdentity(t *testing.T) {
	project := Project{ID: 42, UserID: 7}
	for _, jobID := range []string{"../other", "job/../../other", `job\other`, "*", "job\nother", ".", strings.Repeat("a", 101)} {
		if path, err := project.GetDeploymentLogPath(t.TempDir(), jobID, "build"); err == nil || path != "" {
			t.Fatalf("invalid job ID %q accepted: %q, %v", jobID, path, err)
		}
	}
	if path, err := project.GetDeploymentLogPath(t.TempDir(), "job-1", "../other"); err == nil || path != "" {
		t.Fatalf("invalid log type accepted: %q, %v", path, err)
	}
}
