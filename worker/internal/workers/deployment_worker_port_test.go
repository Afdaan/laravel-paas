package workers

import (
	"testing"

	"github.com/laravel-paas/shared/models"
)

func TestDeploymentCompletionMessage(t *testing.T) {
	workerPort := 0
	workerProject := &models.Project{Port: &workerPort}
	if got := deploymentCompletionMessage(workerProject); got != "Worker-only service deployed successfully" {
		t.Fatalf("worker-only message=%q", got)
	}

	webPort := 8080
	webProject := &models.Project{Port: &webPort}
	if got := deploymentCompletionMessage(webProject); got != "Release successfully promoted and live" {
		t.Fatalf("web message=%q", got)
	}

	unknownProject := &models.Project{Framework: "Custom"}
	if got := deploymentCompletionMessage(unknownProject); got != "Release successfully promoted and live" {
		t.Fatalf("unknown framework completion message=%q", got)
	}
}
