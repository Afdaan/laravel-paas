package nginx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGetSSLStatus_UsesProjectCertificateIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("cert_name"); got != "app.paas.test" {
			t.Fatalf("Expected cert_name app.paas.test, got %q", got)
		}
		if got := r.URL.Query().Get("domain"); got != "custom.example.com" {
			t.Fatalf("Expected domain custom.example.com, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cert_name":"app.paas.test","domain":"custom.example.com","status":"ssl_active"}`))
	}))
	defer server.Close()

	service := NewNginxWebhookService(&config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     server.URL,
		NginxWebhookKey:     "testkey",
	})

	status, err := service.GetSSLStatus("app.paas.test", "custom.example.com")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if status.Status != "ssl_active" {
		t.Fatalf("Expected ssl_active, got %q", status.Status)
	}
}

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

func TestSyncProject_RetriesTransientNetworkFailure(t *testing.T) {
	var attempts atomic.Int32
	service := NewNginxWebhookService(&config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     "http://private-routing.internal/webhook",
		NginxWebhookKey:     "testkey",
	})
	service.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if attempts.Add(1) < webhookMaxAttempts {
			return nil, syscall.ECONNRESET
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"Synced","config_hash":"retry-hash"}`)),
			Request:    req,
		}, nil
	})

	hash, err := service.SyncProject(&models.Project{Subdomain: "testproj"}, "paas.test")
	if err != nil {
		t.Fatalf("Expected retry to recover, got: %v", err)
	}
	if hash != "retry-hash" {
		t.Fatalf("Expected retry-hash, got %q", hash)
	}
	if got := attempts.Load(); got != webhookMaxAttempts {
		t.Fatalf("Expected %d attempts, got %d", webhookMaxAttempts, got)
	}
}

func TestSyncProject_NetworkFailureRemainsSanitized(t *testing.T) {
	var attempts atomic.Int32
	cfg := &config.Config{
		NginxWebhookEnabled: true,
		NginxWebhookURL:     "http://10.0.0.8:49512/webhook",
		NginxWebhookKey:     "testkey",
	}
	service := NewNginxWebhookService(cfg)
	service.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, syscall.ECONNREFUSED
	})

	_, err := service.SyncProject(&models.Project{Subdomain: "testproj"}, "paas.test")
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if !strings.Contains(err.Error(), "webhook connection refused") {
		t.Fatalf("Expected safe connection-refused category, got: %v", err)
	}
	if strings.Contains(err.Error(), cfg.NginxWebhookURL) {
		t.Fatalf("Security risk: error contains webhook URL: %v", err)
	}
	if got := attempts.Load(); got != webhookMaxAttempts {
		t.Fatalf("Expected %d attempts, got %d", webhookMaxAttempts, got)
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
