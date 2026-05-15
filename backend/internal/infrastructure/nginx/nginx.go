package nginx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
)

// NginxWebhookService handles communication with the remote Nginx VM
type NginxWebhookService struct {
	cfg *config.Config
}

// NewNginxWebhookService creates a new nginx webhook service
func NewNginxWebhookService(cfg *config.Config) *NginxWebhookService {
	return &NginxWebhookService{
		cfg: cfg,
	}
}

// WebhookPayload represents the data sent to the Nginx VM
type WebhookPayload struct {
	Action        string   `json:"action"` // "sync" or "delete"
	Subdomain     string   `json:"subdomain"`
	Domain        string   `json:"domain"`
	CustomDomains []string `json:"custom_domains,omitempty"`
	Port          int      `json:"port"`
	InternalIP    string   `json:"internal_ip"`
	UserFolder    string   `json:"user_folder"`
}

// SyncProject sends a sync request to the Nginx webhook
func (s *NginxWebhookService) SyncProject(project *models.Project, domain string) error {
	if !s.cfg.NginxWebhookEnabled || s.cfg.NginxWebhookURL == "" {
		return nil
	}

	port := 80
	if project.Port != nil {
		port = *project.Port
	} else {
		slog.Warn("Project has no port assigned, defaulting to 80 for Nginx sync", "subdomain", project.Subdomain)
	}

	// Determine target configuration directory using a sanitized, traceable identifier.
	userFolder := s.getUserFolderName(project)

	var customDomains []string
	for _, cd := range project.CustomDomains {
		if cd.Status == models.DomainStatusActive {
			customDomains = append(customDomains, cd.Domain)
		}
	}

	payload := WebhookPayload{
		Action:        "sync",
		Subdomain:     project.Subdomain,
		Domain:        project.GetFullDomain(domain),
		CustomDomains: customDomains,
		Port:          port,
		InternalIP:    s.cfg.InternalIP,
		UserFolder:    userFolder,
	}

	return s.sendRequest(payload)
}

// DeleteProject sends a delete request to the Nginx webhook
func (s *NginxWebhookService) DeleteProject(project *models.Project, domain string) error {
	if !s.cfg.NginxWebhookEnabled || s.cfg.NginxWebhookURL == "" {
		return nil
	}

	userFolder := s.getUserFolderName(project)

	payload := WebhookPayload{
		Action:     "delete",
		Subdomain:  project.Subdomain,
		Domain:     project.GetFullDomain(domain),
		UserFolder: userFolder,
	}

	return s.sendRequest(payload)
}

func (s *NginxWebhookService) sendRequest(payload WebhookPayload) error {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	req, err := http.NewRequest("POST", s.cfg.NginxWebhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Key", s.cfg.NginxWebhookKey)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status: %d", resp.StatusCode)
	}

	slog.Info("Successfully sent Nginx webhook", "subdomain", payload.Subdomain, "action", payload.Action)
	return nil
}

// getUserFolderName generates a predictable, POSIX-compliant directory name based on the user's email.
// It strips all special characters and appends the user ID to guarantee uniqueness and prevent directory traversal or collision issues.
func (s *NginxWebhookService) getUserFolderName(project *models.Project) string {
	if project.User.Email == "" {
		return fmt.Sprintf("user-%d", project.UserID)
	}

	emailPrefix := strings.Split(project.User.Email, "@")[0]

	// Sanitize prefix: preserve only alphanumeric characters.
	// Replace any special character with a hyphen to maintain readability.
	emailPrefix = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, emailPrefix)

	// Collapse consecutive hyphens to prevent malformed directory names (e.g., "user--name" -> "user-name").
	for strings.Contains(emailPrefix, "--") {
		emailPrefix = strings.ReplaceAll(emailPrefix, "--", "-")
	}

	// Remove leading and trailing hyphens to ensure valid directory naming constraints.
	emailPrefix = strings.Trim(emailPrefix, "-")

	// Fallback to generic prefix if sanitization stripped the entire string.
	if emailPrefix == "" {
		emailPrefix = "user"
	}

	return fmt.Sprintf("%s-%d", strings.ToLower(emailPrefix), project.UserID)
}
