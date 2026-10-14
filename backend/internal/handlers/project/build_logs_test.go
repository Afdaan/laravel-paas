package project

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/models"
)

func TestTailLines(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		n            int
		partialFirst bool
		want         string
	}{
		{
			name:         "drops the leading fragment of a mid-file window",
			input:        "ent of a line\nsecond\nthird\n",
			n:            5,
			partialFirst: true,
			want:         "second\nthird",
		},
		{
			name:  "returns everything when fewer lines exist than requested",
			input: "one\ntwo\n",
			n:     10,
			want:  "one\ntwo",
		},
		{
			name:  "a trailing newline produces no empty final line",
			input: "one\ntwo\nthree\n",
			n:     2,
			want:  "two\nthree",
		},
		{
			name:         "a whole-file window keeps its first line",
			input:        "one\ntwo\n",
			n:            5,
			partialFirst: false,
			want:         "one\ntwo",
		},
		{
			name:         "a window with no line break is kept as a truncated line",
			input:        "no break at all",
			n:            3,
			partialFirst: true,
			want:         "no break at all",
		},
		{
			name:  "an empty file yields an empty tail",
			input: "",
			n:     6,
			want:  "",
		},
		{
			name:  "a file of only newlines yields an empty tail",
			input: "\n\n\n",
			n:     6,
			want:  "",
		},
		{
			name:  "n of zero leaves the input untouched",
			input: "one\ntwo\n",
			n:     0,
			want:  "one\ntwo\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tailLines(tc.input, tc.n, tc.partialFirst); got != tc.want {
				t.Fatalf("tailLines(%q, %d, %v) = %q, want %q", tc.input, tc.n, tc.partialFirst, got, tc.want)
			}
		})
	}
}

// The clamp is what keeps the read window bounded; it lives in the handler, so
// pin the arithmetic that depends on it.
func TestTailReadWindowStaysUnderResponseCap(t *testing.T) {
	const maxBytes = 256 * 1024
	if maxTailLines*bytesPerTailLine+tailReadFloor > maxBytes {
		t.Fatalf("a clamped tail request may read %d bytes, above the %d cap",
			maxTailLines*bytesPerTailLine+tailReadFloor, maxBytes)
	}
}

func TestGetActiveOrLatestLogPathFallbackToBuildLog(t *testing.T) {
	dir := t.TempDir()
	legacyFile := filepath.Join(dir, "build.log")
	if err := os.WriteFile(legacyFile, []byte("legacy output"), 0644); err != nil {
		t.Fatal(err)
	}

	got := getActiveOrLatestLogPath(dir, "")
	if got != legacyFile {
		t.Fatalf("getActiveOrLatestLogPath() = %q, want %q", got, legacyFile)
	}
	if got := getActiveOrLatestLogPath(dir, "job-missing"); got == legacyFile {
		t.Fatal("active job received unversioned legacy log")
	}
}

func TestBuildLogsRejectsStaleFallbackLog(t *testing.T) {
	handler, project, user, db, _, _ := setupRequeueTest(t, nil)
	handler.cfg.ProjectsPath = t.TempDir()

	projectPath := project.GetProjectPath(handler.cfg.ProjectsPath)
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatal(err)
	}

	buildLogPath := filepath.Join(projectPath, "build.log")
	if err := os.WriteFile(buildLogPath, []byte("stale output from old run\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set file ModTime to 1 hour ago
	pastTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(buildLogPath, pastTime, pastTime); err != nil {
		t.Fatal(err)
	}

	// Active job enqueued 10 seconds ago
	enqueuedAt := time.Now().Add(-10 * time.Second)
	jobID := "job-active-123"
	if err := db.Model(&project).Updates(map[string]any{
		"deployment_job_id":      jobID,
		"deployment_status":      models.DepStatusBuilding,
		"deployment_enqueued_at": enqueuedAt,
	}).Error; err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Get("/projects/:id/build-logs", func(ctx *fiber.Ctx) error {
		ctx.Locals("user_id", user.ID)
		ctx.Locals("role", string(user.Role))
		return handler.BuildLogs(ctx)
	})

	req := httptest.NewRequest("GET", "/projects/"+project.UID+"/build-logs", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var payload struct {
		Logs        string `json:"logs"`
		Available   bool   `json:"available"`
		Placeholder bool   `json:"placeholder"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}

	if payload.Available || !payload.Placeholder || payload.Logs != "Initializing build environment..." {
		t.Fatalf("expected placeholder for stale fallback, got: %+v", payload)
	}

	// Now touch file so it is newer than enqueuedAt
	freshTime := time.Now()
	if err := os.Chtimes(buildLogPath, freshTime, freshTime); err != nil {
		t.Fatal(err)
	}

	req2 := httptest.NewRequest("GET", "/projects/"+project.UID+"/build-logs", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	var payload2 struct {
		Logs        string `json:"logs"`
		Available   bool   `json:"available"`
		Placeholder bool   `json:"placeholder"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&payload2); err != nil {
		t.Fatal(err)
	}

	if payload2.Available || !payload2.Placeholder {
		t.Fatalf("fresh timestamp must not establish log ownership, got: %+v", payload2)
	}
}

func TestResolveLogPathWithUserDirMismatch(t *testing.T) {
	handler, project, _, _, _, _ := setupRequeueTest(t, nil)
	tmpDir := t.TempDir()
	handler.cfg.ProjectsPath = tmpDir

	actualDir := filepath.Join(tmpDir, models.GetUserDirName(handler.db, project.UserID), project.Subdomain, "logs")
	if err := os.MkdirAll(actualDir, 0755); err != nil {
		t.Fatal(err)
	}
	expectedLog := filepath.Join(actualDir, "build-job-xyz.log")
	if err := os.WriteFile(expectedLog, []byte("build content"), 0644); err != nil {
		t.Fatal(err)
	}

	project.UserSlug = "user-unknown"
	resolved := handler.resolveLogPath(&project, "job-xyz")
	if resolved != expectedLog {
		t.Fatalf("resolveLogPath() = %q, want %q", resolved, expectedLog)
	}
}

func TestBuildLogsReadsCanonicalJobLog(t *testing.T) {
	handler, project, user, db, _, _ := setupRequeueTest(t, nil)
	handler.cfg.ProjectsPath = t.TempDir()
	jobID := "job-current"
	if err := db.Model(&project).Update("deployment_job_id", jobID).Error; err != nil {
		t.Fatal(err)
	}
	logPath, err := project.GetDeploymentLogPath(handler.cfg.ProjectsPath, jobID, "build")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("prepare\ncompile\ncomplete\n"); err != nil {
		t.Fatal(err)
	}
	pastTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(logPath, pastTime, pastTime); err != nil {
		t.Fatal(err)
	}
	app := fiber.New()
	app.Get("/projects/:id/build-logs", func(ctx *fiber.Ctx) error {
		ctx.Locals("user_id", user.ID)
		ctx.Locals("role", string(models.RoleUser))
		return handler.BuildLogs(ctx)
	})
	response, err := app.Test(httptest.NewRequest("GET", "/projects/"+project.UID+"/build-logs?tail=2", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("sensitive logs can be cached: Cache-Control=%q", got)
	}
	var payload struct {
		Logs        string `json:"logs"`
		JobID       string `json:"job_id"`
		Available   bool   `json:"available"`
		Placeholder bool   `json:"placeholder"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK || payload.Logs != "compile\ncomplete" || payload.JobID != jobID || !payload.Available || payload.Placeholder {
		t.Fatalf("unexpected build logs: status=%d payload=%+v", response.StatusCode, payload)
	}
	app.Get("/other/projects/:id/build-logs", func(ctx *fiber.Ctx) error {
		ctx.Locals("user_id", user.ID+1)
		ctx.Locals("role", string(models.RoleUser))
		return handler.BuildLogs(ctx)
	})
	denied, err := app.Test(httptest.NewRequest("GET", "/other/projects/"+project.UID+"/build-logs", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != fiber.StatusNotFound {
		t.Fatalf("another tenant received build logs: status=%d", denied.StatusCode)
	}
}

func TestResolveLogPathRejectsForeignAndStaleLogs(t *testing.T) {
	handler, project, _, _, _, _ := setupRequeueTest(t, nil)
	handler.cfg.ProjectsPath = t.TempDir()
	jobID := "job-current"
	foreign := project
	foreign.UserID++
	foreign.ID++
	foreign.UserSlug = fmt.Sprintf("other-%d", foreign.UserID)
	foreignLog, err := foreign.GetDeploymentLogPath(handler.cfg.ProjectsPath, jobID, "build")
	if err != nil {
		t.Fatal(err)
	}
	foreignSourceLog := filepath.Join(foreign.GetProjectPath(handler.cfg.ProjectsPath), "logs", "build-"+jobID+".log")
	oldLog, err := project.GetDeploymentLogPath(handler.cfg.ProjectsPath, "job-previous", "build")
	if err != nil {
		t.Fatal(err)
	}
	for _, logPath := range []string{foreignLog, foreignSourceLog, oldLog} {
		if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(logPath, []byte("must not be served"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := handler.resolveLogPath(&project, jobID); got != "" {
		t.Fatalf("foreign or stale log selected: %q", got)
	}
	currentLog, err := project.GetDeploymentLogPath(handler.cfg.ProjectsPath, jobID, "build")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignLog, currentLog); err != nil {
		t.Fatal(err)
	}
	if got := handler.resolveLogPath(&project, jobID); got != "" {
		t.Fatalf("foreign symlink selected: %q", got)
	}
	legacyLog := filepath.Join(project.GetProjectPath(handler.cfg.ProjectsPath), "logs", "build-"+jobID+".log")
	if err := os.MkdirAll(filepath.Dir(legacyLog), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(foreignLog, legacyLog); err != nil {
		t.Fatal(err)
	}
	if got := handler.resolveLogPath(&project, jobID); got != "" {
		t.Fatalf("foreign legacy symlink selected: %q", got)
	}
	if got := handler.resolveLogPath(&project, "../../foreign"); got != "" {
		t.Fatalf("invalid job ID selected: %q", got)
	}
	if err := os.RemoveAll(filepath.Dir(currentLog)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(foreignLog), filepath.Dir(currentLog)); err != nil {
		t.Fatal(err)
	}
	if got := handler.resolveLogPath(&project, jobID); got != "" {
		t.Fatalf("foreign log directory selected: %q", got)
	}
}
