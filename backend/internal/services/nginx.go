package services

	import (
		"bytes"
		"encoding/json"
		"fmt"
		"log/slog"
		"net/http"
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
		Action     string `json:"action"` // "sync" or "delete"
		Subdomain  string `json:"subdomain"`
		Domain     string `json:"domain"`
		Port       int    `json:"port"`
		InternalIP string `json:"internal_ip"`
		UserFolder string `json:"user_folder"`
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

		// Use stable User ID for folder naming to prevent orphans/conflicts
		userFolder := fmt.Sprintf("user-%d", project.UserID)

		payload := WebhookPayload{
			Action:     "sync",
			Subdomain:  project.Subdomain,
			Domain:     project.GetFullDomain(domain),
			Port:       port,
			InternalIP: s.cfg.InternalIP,
			UserFolder: userFolder,
		}

		return s.sendRequest(payload)
	}

	// DeleteProject sends a delete request to the Nginx webhook
	func (s *NginxWebhookService) DeleteProject(project *models.Project, domain string) error {
		if !s.cfg.NginxWebhookEnabled || s.cfg.NginxWebhookURL == "" {
			return nil
		}

		userFolder := fmt.Sprintf("user-%d", project.UserID)

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
			Timeout: 10 * time.Second,
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
	
