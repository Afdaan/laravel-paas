package nginx

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
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

	// Since project containers are on a private Docker network and not exposed directly,
	// Nginx must always proxy incoming requests to the Traefik reverse proxy running on the host.
	port := s.cfg.TraefikHTTPPort

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
	CertName   string `json:"cert_name"`
	Domain     string `json:"domain"`
	Status     string `json:"status"`
	Error      string `json:"error"`
	IssuedAt   string `json:"issued_at"`
	ExpiresAt  string `json:"expires_at"`
	RetryCount int    `json:"retry_count"`
}

// GetSSLStatus queries the remote Nginx VM for Let's Encrypt certificate issuance status and valid dates.
func (s *NginxWebhookService) GetSSLStatus(certName, domain string) (*SSLStatusResponse, error) {
	if !s.cfg.NginxWebhookEnabled || s.cfg.NginxWebhookURL == "" {
		return s.GetSSLStatusFromTLS(domain)
	}

	baseURL := strings.TrimSuffix(s.cfg.NginxWebhookURL, "/webhook")
	query := url.Values{}
	query.Set("cert_name", certName)
	query.Set("domain", domain)
	targetURL := fmt.Sprintf("%s/ssl-status?%s", baseURL, query.Encode())

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

// GetSSLStatusFromTLS attempts to establish a real TLS connection to the domain and parse its certificate dates.
func (s *NginxWebhookService) GetSSLStatusFromTLS(domain string) (*SSLStatusResponse, error) {
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", domain+":443", &tls.Config{
		ServerName: domain,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to dial tls: %w", err)
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found")
	}

	leaf := certs[0]
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return &SSLStatusResponse{
			Domain: domain,
			Status: "failed",
			Error:  "certificate is not valid yet",
		}, nil
	}
	if now.After(leaf.NotAfter) {
		return &SSLStatusResponse{
			Domain: domain,
			Status: "expired",
			Error:  "certificate has expired",
		}, nil
	}

	const timeLayout = "Jan 2 15:04:05 2006 MST"
	return &SSLStatusResponse{
		Domain:    domain,
		Status:    "ssl_active",
		IssuedAt:  leaf.NotBefore.Format(timeLayout),
		ExpiresAt: leaf.NotAfter.Format(timeLayout),
	}, nil
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
		slog.Error("Nginx webhook request failed with raw details", "subdomain", payload.Subdomain, "error", err)
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return "", fmt.Errorf("webhook request timed out")
		}
		return "", fmt.Errorf("webhook request network failure")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Error("Nginx webhook returned non-2xx status", "subdomain", payload.Subdomain, "status", resp.StatusCode)
		return "", fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	var webhookResp WebhookResponse
	if err := json.NewDecoder(resp.Body).Decode(&webhookResp); err != nil {
		slog.Error("Failed to decode Nginx webhook response", "subdomain", payload.Subdomain, "error", err)
		return "", fmt.Errorf("invalid webhook response format")
	}
	if payload.Action == "sync" && webhookResp.ConfigHash == "" {
		slog.Error("Nginx webhook returned empty config hash", "subdomain", payload.Subdomain)
		return "", fmt.Errorf("empty config hash returned from webhook")
	}

	slog.Info("Successfully sent Nginx webhook", "subdomain", payload.Subdomain, "action", payload.Action, "hash", webhookResp.ConfigHash)
	return webhookResp.ConfigHash, nil
}

// getUserFolderName generates a predictable, POSIX-compliant directory name based on the user's email or UserSlug.
func (s *NginxWebhookService) getUserFolderName(project *models.Project) string {
	if project.UserSlug != "" && project.UserSlug != "user-unknown" {
		return project.UserSlug
	}

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
