package nginx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

func TestSyncProject_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Synced", "config_hash": "abc123hash"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     server.URL,
		NginxWebhookKey:     "testkey",
	}

	service := NewNginxWebhookService(cfg)
	project := &models.Project{
		Subdomain: "testproj",
	}

	hash, err := service.SyncProject(project, "paas.test")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if hash != "abc123hash" {
		t.Errorf("Expected hash 'abc123hash', got '%s'", hash)
	}
}

func TestSyncProject_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := &config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     server.URL,
		NginxWebhookKey:     "testkey",
	}

	service := NewNginxWebhookService(cfg)
	project := &models.Project{
		Subdomain: "testproj",
	}

	_, err := service.SyncProject(project, "paas.test")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "webhook returned HTTP 500") {
		t.Errorf("Expected sanitized error containing HTTP 500, got: %v", err)
	}
	// Verify we do NOT leak internal URL details
	if strings.Contains(err.Error(), server.URL) {
		t.Errorf("Security risk: error message contains raw webhook URL: %v", err)
	}
}

func TestSyncProject_EmptyHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Synced but no hash"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     server.URL,
		NginxWebhookKey:     "testkey",
	}

	service := NewNginxWebhookService(cfg)
	project := &models.Project{
		Subdomain: "testproj",
	}

	_, err := service.SyncProject(project, "paas.test")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "empty config hash returned from webhook") {
		t.Errorf("Expected error to mention empty config hash, got: %v", err)
	}
}

func TestDeleteProject_SuccessWithoutHash(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "Deleted successfully"}`))
	}))
	defer server.Close()

	cfg := &config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     server.URL,
		NginxWebhookKey:     "testkey",
	}

	service := NewNginxWebhookService(cfg)
	project := &models.Project{
		Subdomain: "testproj",
	}

	err := service.DeleteProject(project, "paas.test")
	if err != nil {
		t.Fatalf("Expected no error on delete, got: %v", err)
	}
}
