package project

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

func TestGetProjectReturnsApplicationClock(t *testing.T) {
	handler, project, user, db, _, _ := setupRequeueTest(t, nil)
	enqueuedAt := time.Now().UTC().Add(-36 * time.Second)
	if err := db.Model(&project).Update("deployment_enqueued_at", enqueuedAt).Error; err != nil {
		t.Fatal(err)
	}

	app := fiber.New()
	app.Get("/projects/:id", func(ctx *fiber.Ctx) error {
		ctx.Locals("user_id", user.ID)
		ctx.Locals("role", string(user.Role))
		return handler.Get(ctx)
	})

	before := time.Now().UTC()
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/projects/"+project.UID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	after := time.Now().UTC()
	if response.StatusCode != http.StatusOK || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response: %d, %v", response.StatusCode, response.Header)
	}

	var payload struct {
		UID        string    `json:"uid"`
		ServerTime time.Time `json:"server_time"`
		EnqueuedAt time.Time `json:"deployment_enqueued_at"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.UID != project.UID || !payload.EnqueuedAt.Equal(enqueuedAt) {
		t.Fatalf("project payload changed: %+v", payload)
	}
	if payload.ServerTime.Before(before) || payload.ServerTime.After(after) {
		t.Fatalf("server_time %s outside application clock bounds", payload.ServerTime)
	}
}
