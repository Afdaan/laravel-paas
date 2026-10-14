package infrastructure

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestCheckoutCommitPinsRollbackTargetAndRejectsMissingCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	repository := filepath.Join(root, "origin")
	clone := filepath.Join(root, "clone")
	run := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("init", repository)
	run("-C", repository, "config", "user.email", "test@example.com")
	run("-C", repository, "config", "user.name", "Test")
	file := filepath.Join(repository, "app.txt")
	if err := os.WriteFile(file, []byte("previous image"), 0644); err != nil {
		t.Fatal(err)
	}
	run("-C", repository, "add", "app.txt")
	run("-C", repository, "commit", "-m", "previous")
	previousHash := run("-C", repository, "rev-parse", "HEAD")
	if err := os.WriteFile(file, []byte("new branch tip"), 0644); err != nil {
		t.Fatal(err)
	}
	run("-C", repository, "commit", "-am", "new")
	run("clone", "--depth=1", "file://"+repository, clone)
	service := &GitService{}
	commit, err := service.CheckoutCommit(clone, "file://"+repository, previousHash)
	if err != nil || commit != previousHash {
		t.Fatalf("rollback checkout: commit=%s err=%v", commit, err)
	}
	content, err := os.ReadFile(filepath.Join(clone, "app.txt"))
	if err != nil || string(content) != "previous image" {
		t.Fatalf("wrong source after rollback checkout: %q err=%v", content, err)
	}
	if _, err := service.CheckoutCommit(clone, "file://"+repository, strings.Repeat("f", 40)); err == nil {
		t.Fatal("missing rollback target accepted")
	}
	if current := run("-C", clone, "rev-parse", "HEAD"); current != previousHash {
		t.Fatalf("failed fallback changed active source: %s", current)
	}
}

func TestDeploymentLogSurvivesRepositorySyncAndReplacement(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	for _, mode := range []string{"sync", "fresh-clone"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			repository := filepath.Join(root, "origin")
			run := func(args ...string) {
				t.Helper()
				if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
					t.Fatalf("git %v: %v: %s", args, err, output)
				}
			}
			run("init", "-b", "main", repository)
			if err := os.WriteFile(filepath.Join(repository, "app.txt"), []byte("source"), 0600); err != nil {
				t.Fatal(err)
			}
			run("-C", repository, "add", "app.txt")
			run("-C", repository, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "initial")
			cfg := &config.Config{ProjectsPath: filepath.Join(root, "projects")}
			service := NewGitService(cfg, nil)
			project := models.Project{ID: 42, UserID: 7, Subdomain: "app"}
			source, _, err := service.CloneRepository(project.UserID, "file://"+repository, "main", project.Subdomain)
			if err != nil {
				t.Fatal(err)
			}
			logPath, err := project.GetDeploymentLogPath(cfg.ProjectsPath, "job-current", "build")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
				t.Fatal(err)
			}
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer logFile.Close()
			if _, err := logFile.WriteString("before clone\n"); err != nil {
				t.Fatal(err)
			}
			legacyLog := filepath.Join(source, "logs", "build-job-current.log")
			if err := os.MkdirAll(filepath.Dir(legacyLog), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(legacyLog, []byte("old layout"), 0600); err != nil {
				t.Fatal(err)
			}
			if mode == "fresh-clone" {
				if err := os.RemoveAll(filepath.Join(source, ".git")); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := service.CloneRepository(project.UserID, "file://"+repository, "main", project.Subdomain); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(legacyLog); !os.IsNotExist(err) {
				t.Fatalf("test did not exercise source cleanup: %v", err)
			}
			if _, err := logFile.WriteString("after clone\n"); err != nil {
				t.Fatal(err)
			}
			output, err := os.ReadFile(logPath)
			if err != nil || string(output) != "before clone\nafter clone\n" {
				t.Fatalf("active log lost during %s: %q, %v", mode, output, err)
			}
		})
	}
}
