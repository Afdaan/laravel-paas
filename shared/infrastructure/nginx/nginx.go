package nginx

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
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

// WebhookResponse represents the JSON response returned by the remote Nginx VM
type WebhookResponse struct {
	Message    string `json:"message"`
	ConfigHash string `json:"config_hash"`
}

// signRequest generates an HMAC SHA-256 signature using timestamp, nonce, method, path, and body to prevent replay attacks and tampering.
func (s *NginxWebhookService) signRequest(req *http.Request, bodyBytes []byte) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := utils.GenerateRandomUID()
	path := req.URL.Path
	if req.URL.RawQuery != "" {
		path = path + "?" + req.URL.RawQuery
	}
	rawStr := fmt.Sprintf("%s:%s:%s:%s:%s", timestamp, nonce, req.Method, path, string(bodyBytes))

	h := hmac.New(sha256.New, []byte(s.cfg.NginxWebhookKey))
	h.Write([]byte(rawStr))
	sig := hex.EncodeToString(h.Sum(nil))

	req.Header.Set("X-Webhook-Timestamp", timestamp)
	req.Header.Set("X-Webhook-Nonce", nonce)
	req.Header.Set("X-Webhook-Signature", sig)
	req.Header.Set("X-Webhook-Key", s.cfg.NginxWebhookKey)
}

// SyncProject sends a sync request to the Nginx webhook and returns the resulting SHA256 config hash.
func (s *NginxWebhookService) SyncProject(project *models.Project, domain string) (string, error) {
	if !s.cfg.NginxWebhookEnabled || s.cfg.NginxWebhookURL == "" {
		return "", nil
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
		if cd.Domain != "" && models.IsNginxRoutableCustomDomainStatus(cd.Status) {
			customDomains = append(customDomains, cd.Domain)
		}
	}

	serverNames := append([]string{project.GetFullDomain(domain)}, customDomains...)
	slog.Info("Prepared Nginx render state",
		"projectID", project.ID,
		"subdomain", project.Subdomain,
		"loadedCustomDomainCount", len(project.CustomDomains),
		"verifiedCustomDomains", customDomains,
		"serverNames", serverNames)

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

	_, err := s.sendRequest(payload)
	return err
}

// SSLStatusResponse represents the JSON payload returned by the remote Nginx VM /ssl-status endpoint.
type SSLStatusResponse struct {
	Domain     string `json:"domain"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	IssuedAt   string `json:"issued_at"`
	ExpiresAt  string `json:"expires_at"`
	RetryCount int    `json:"retry_count"`
}

// GetSSLStatus queries the remote Nginx VM for Let's Encrypt certificate issuance status and valid dates.
func (s *NginxWebhookService) GetSSLStatus(domain string) (*SSLStatusResponse, error) {
	if !s.cfg.NginxWebhookEnabled || s.cfg.NginxWebhookURL == "" {
		return nil, fmt.Errorf("nginx webhook not configured")
	}

	baseURL := strings.TrimSuffix(s.cfg.NginxWebhookURL, "/webhook")
	targetURL := fmt.Sprintf("%s/ssl-status?domain=%s", baseURL, domain)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, err
	}
	s.signRequest(req, nil)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ssl status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ssl status endpoint returned HTTP %d", resp.StatusCode)
	}

	var sslResp SSLStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sslResp); err != nil {
		return nil, fmt.Errorf("failed to decode ssl status response: %w", err)
	}

	return &sslResp, nil
}

func (s *NginxWebhookService) sendRequest(payload WebhookPayload) (string, error) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: 120 * time.Second,
	}

	req, err := http.NewRequest("POST", s.cfg.NginxWebhookURL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	s.signRequest(req, jsonPayload)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("webhook returned status: %d", resp.StatusCode)
	}

	var webhookResp WebhookResponse
	_ = json.NewDecoder(resp.Body).Decode(&webhookResp)

	slog.Info("Successfully sent Nginx webhook", "subdomain", payload.Subdomain, "action", payload.Action, "hash", webhookResp.ConfigHash)
	return webhookResp.ConfigHash, nil
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
