package infrastructure

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
