package domain

import (
	"bufio"
	"encoding/json"
	"fmt"
	mathrand "math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/services/domain"
	"github.com/laravel-paas/shared/pkg/traefik"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
	"gorm.io/gorm"
)

type DomainHandler struct {
	cfg            *config.Config
	db             *gorm.DB
	redisService   *infrastructure.RedisService
	domainService  *domain.DomainService
	projectService *projectServicePkg.ProjectService
}

func NewDomainHandler(cfg *config.Config, db *gorm.DB, redisService *infrastructure.RedisService, domainService *domain.DomainService, projectService *projectServicePkg.ProjectService) *DomainHandler {
	return &DomainHandler{
		cfg:            cfg,
		db:             db,
		redisService:   redisService,
		domainService:  domainService,
		projectService: projectService,
	}
}

// validateAccess verifies that the authenticated user owns the project or has administrative privileges.
func (h *DomainHandler) validateAccess(c *fiber.Ctx, projectID uint, domainID uint) (*models.Project, *models.CustomDomain, error) {
	project, err := h.projectService.GetProjectByID(projectID)
	if err != nil {
		return nil, nil, apperr.New(404, "PROJECT_NOT_FOUND", "Project not found")
	}

	userID := c.Locals("user_id").(uint)
	role := c.Locals("role").(string)
	if project.UserID != userID && role != string(models.RoleSuperAdmin) && role != string(models.RoleAdmin) {
		return nil, nil, apperr.New(403, "FORBIDDEN", "You do not have permission to access this project")
	}

	if domainID == 0 {
		return project, nil, nil
	}

	domain, err := h.domainService.GetDomainByID(domainID)
	if err != nil {
		return nil, nil, apperr.New(404, "DOMAIN_NOT_FOUND", "Domain not found")
	}

	if domain.ProjectID != projectID {
		return nil, nil, apperr.New(400, "MISMATCH", "Domain does not belong to the specified project")
	}

	return project, domain, nil
}

// RegisterRoutes registers the domain endpoints to a router
func (h *DomainHandler) RegisterRoutes(r fiber.Router) {
	r.Get("", h.List)
	r.Post("", h.Add)
	r.Delete("/:domainID", h.Remove)
	r.Post("/:domainID/verify", h.Verify)
	r.Get("/:domainID/diagnostic", h.Diagnostic)
	r.Post("/:domainID/transfer", h.Transfer)
	r.Get("/:domainID/events", h.ListEvents)
	r.Get("/:domainID/events/stream", h.StreamEvents)
	r.Get("/events/stream", h.StreamProjectEvents)
}

func (h *DomainHandler) List(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	if _, _, err := h.validateAccess(c, uint(projectID), 0); err != nil {
		return err
	}

	domains, err := h.domainService.ListDomains(uint(projectID))
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": domains,
	})
}

func (h *DomainHandler) ListAll(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	domains, err := h.domainService.ListUserDomains(userID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": domains,
	})
}

func (h *DomainHandler) ListGlobal(c *fiber.Ctx) error {
	domains, err := h.domainService.ListAllDomains()
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": domains,
	})
}

func (h *DomainHandler) ListGlobalMetrics(c *fiber.Ctx) error {
	metrics, err := h.domainService.GetMetrics()
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": metrics,
	})
}

func (h *DomainHandler) Add(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	if _, _, err := h.validateAccess(c, uint(projectID), 0); err != nil {
		return err
	}

	var req struct {
		Domain string `json:"domain"`
	}

	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "INVALID_REQUEST", "Invalid request body")
	}

	domainData, err := h.domainService.AddDomain(uint(projectID), req.Domain)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": domainData,
	})
}

func (h *DomainHandler) Remove(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil || domainID == 0 {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	project, _, err := h.validateAccess(c, uint(projectID), uint(domainID))
	if err != nil {
		return err
	}

	if err := h.domainService.RemoveDomain(uint(domainID), uint(projectID)); err != nil {
		return err
	}

	// Trigger Nginx & Traefik Sync using a fresh, authoritative database snapshot (without the removed domain)
	updatedProject, err := h.projectService.GetProjectByID(uint(projectID))
	if err == nil {
		_, _ = h.projectService.SyncProjectNginxFrom(updatedProject, "domain_remove_handler")
		_ = traefik.WriteProjectDynamicFile(h.cfg, updatedProject, updatedProject.CustomDomains)
	} else if project != nil {
		_, _ = h.projectService.SyncProjectNginxFrom(project, "domain_remove_handler_fallback")
		_ = traefik.WriteProjectDynamicFile(h.cfg, project, project.CustomDomains)
	}

	return c.JSON(fiber.Map{"message": "Domain removed successfully"})
}

func (h *DomainHandler) Verify(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil || domainID == 0 {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	project, domain, err := h.validateAccess(c, uint(projectID), uint(domainID))
	if err != nil {
		return err
	}

	wasAlreadyActive := domain != nil && domain.Status == models.DomainStatusActive

	domainData, err := h.domainService.VerifyDomain(c.UserContext(), uint(domainID), uint(projectID), project)
	if err != nil {
		return c.JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "VERIFICATION_FAILED",
				"message": err.Error(),
			},
			"domain": domainData,
		})
	}

	// Trigger Nginx & Traefik Sync only if the domain wasn't already active to avoid redundant downtime/restarts
	if !wasAlreadyActive {
		updatedProject, err := h.projectService.GetProjectByID(uint(projectID))
		if err == nil {
			_, _ = h.projectService.SyncProjectNginxFrom(updatedProject, "domain_verify_handler")
			_ = traefik.WriteProjectDynamicFile(h.cfg, updatedProject, updatedProject.CustomDomains)
		} else {
			_, _ = h.projectService.SyncProjectNginxFrom(project, "domain_verify_handler_fallback")
			_ = traefik.WriteProjectDynamicFile(h.cfg, project, project.CustomDomains)
		}
	}

	return c.JSON(fiber.Map{
		"data": domainData,
	})
}

func (h *DomainHandler) Transfer(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil || domainID == 0 {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	if _, _, err := h.validateAccess(c, uint(projectID), uint(domainID)); err != nil {
		return err
	}

	var req struct {
		TargetProjectID uint `json:"target_project_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "INVALID_REQUEST", "Invalid request body")
	}

	if _, _, err := h.validateAccess(c, req.TargetProjectID, 0); err != nil {
		return apperr.New(403, "FORBIDDEN", "You do not have permission to transfer domains to the target project")
	}

	userID := c.Locals("user_id").(uint)
	if err := h.domainService.TransferDomain(userID, uint(domainID), req.TargetProjectID); err != nil {
		return err
	}

	// Trigger Traefik Sync using fresh, authoritative database snapshots for both source and target projects
	sourceProject, errSource := h.projectService.GetProjectByID(uint(projectID))
	if errSource == nil {
		_ = traefik.WriteProjectDynamicFile(h.cfg, sourceProject, sourceProject.CustomDomains)
	}

	targetProject, errTarget := h.projectService.GetProjectByID(uint(req.TargetProjectID))
	if errTarget == nil {
		_ = traefik.WriteProjectDynamicFile(h.cfg, targetProject, targetProject.CustomDomains)
	}

	return c.JSON(fiber.Map{"message": "Domain transferred successfully"})
}

func (h *DomainHandler) Diagnostic(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil || domainID == 0 {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	project, domain, err := h.validateAccess(c, uint(projectID), uint(domainID))
	if err != nil {
		return err
	}

	diagnostic, err := h.domainService.GetDomainDiagnostic(domain.Domain, project)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": diagnostic,
	})
}

func (h *DomainHandler) ListEvents(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil || domainID == 0 {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	if _, _, err := h.validateAccess(c, uint(projectID), uint(domainID)); err != nil {
		return err
	}

	events, err := h.domainService.ListEvents(uint(domainID))
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": events,
	})
}

func (h *DomainHandler) setSecureCORS(c *fiber.Ctx) {
	allowedOrigin := h.cfg.FrontendURL
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:5173"
	}
	c.Set("Access-Control-Allow-Origin", allowedOrigin)
	c.Set("Access-Control-Allow-Credentials", "true")
	c.Set("Vary", "Origin")
}

func (h *DomainHandler) StreamEvents(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil || domainID == 0 {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	if _, _, err := h.validateAccess(c, uint(projectID), uint(domainID)); err != nil {
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")
	h.setSecureCORS(c)

	lastEventIDStr := c.Get("Last-Event-ID")
	if lastEventIDStr == "" {
		lastEventIDStr = c.Query("last_event_id")
	}
	var lastSeq int
	if lastEventIDStr != "" {
		lastSeq, _ = strconv.Atoi(lastEventIDStr)
	}

	ctx := c.Context()
	eventChan, err := h.domainService.SubscribeEvents(ctx, uint(domainID))
	if err != nil {
		return err
	}

	metrics.GetCollector().IncrSSEConnections()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer metrics.GetCollector().DecrSSEConnections()
		_, _ = w.WriteString("event: connected\ndata: {\"status\":\"connected\"}\n\n")
		_ = w.Flush()

		if lastSeq > 0 {
			metrics.GetCollector().IncrSSEReplayTotal()
			missedEvents, err := h.domainService.ListEventsAfterSequence(uint(domainID), lastSeq)
			if err == nil {
				for _, ev := range missedEvents {
					evBytes, _ := json.Marshal(ev)
					_, _ = w.WriteString(fmt.Sprintf("id: %d\nevent: domain_event\ndata: %s\n\n", ev.SequenceNumber, evBytes))
					lastSeq = ev.SequenceNumber
				}
				_ = w.Flush()
			}
		}

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := w.WriteString("event: heartbeat\ndata: {}\n\n")
				if err != nil {
					return
				}
				_ = w.Flush()
			case msg, ok := <-eventChan:
				if !ok {
					return
				}
				var payload struct {
					SequenceNumber int    `json:"sequence_number"`
					EventType      string `json:"event_type"`
				}
				_ = json.Unmarshal([]byte(msg), &payload)
				if payload.EventType == "redis_reconnected" {
					if lastSeq > 0 {
						time.Sleep(time.Duration(mathrand.Int64N(500)) * time.Millisecond)
						metrics.GetCollector().IncrSSEReplayTotal()
						missedEvents, err := h.domainService.ListEventsAfterSequence(uint(domainID), lastSeq)
						if err == nil {
							for _, ev := range missedEvents {
								evBytes, _ := json.Marshal(ev)
								_, _ = w.WriteString(fmt.Sprintf("id: %d\nevent: domain_event\ndata: %s\n\n", ev.SequenceNumber, evBytes))
								lastSeq = ev.SequenceNumber
							}
							_ = w.Flush()
						}
					}
					continue
				}
				if payload.EventType == "overflow" {
					metrics.GetCollector().IncrSSEOverflowTotal()
					_, _ = w.WriteString("event: overflow\ndata: {\"error\":\"overflow\",\"message\":\"Subscriber buffer overflow, forcing reconnect and replay\"}\n\n")
					_ = w.Flush()
					return
				}
				if payload.SequenceNumber > 0 {
					lastSeq = payload.SequenceNumber
					_, _ = w.WriteString(fmt.Sprintf("id: %d\n", payload.SequenceNumber))
				}
				_, err := w.WriteString(fmt.Sprintf("event: domain_event\ndata: %s\n\n", msg))
				if err != nil {
					return
				}
				_ = w.Flush()
			}
		}
	})

	return nil
}

func (h *DomainHandler) StreamProjectEvents(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil || projectID == 0 {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
	}

	if _, _, err := h.validateAccess(c, uint(projectID), 0); err != nil {
		return err
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")
	h.setSecureCORS(c)

	lastEventIDStr := c.Get("Last-Event-ID")
	if lastEventIDStr == "" {
		lastEventIDStr = c.Query("last_event_id")
	}
	var lastSeq int
	if lastEventIDStr != "" {
		lastSeq, _ = strconv.Atoi(lastEventIDStr)
	}

	ctx := c.Context()
	eventChan, err := h.domainService.SubscribeProjectEvents(ctx, uint(projectID))
	if err != nil {
		return err
	}

	metrics.GetCollector().IncrSSEConnections()

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer metrics.GetCollector().DecrSSEConnections()
		_, _ = w.WriteString("event: connected\ndata: {\"status\":\"connected\"}\n\n")
		_ = w.Flush()

		if lastSeq > 0 {
			metrics.GetCollector().IncrSSEReplayTotal()
			missedEvents, err := h.domainService.ListProjectEventsAfterSequence(uint(projectID), lastSeq)
			if err == nil {
				for _, ev := range missedEvents {
					evBytes, _ := json.Marshal(ev)
					_, _ = w.WriteString(fmt.Sprintf("id: %d\nevent: domain_event\ndata: %s\n\n", ev.SequenceNumber, evBytes))
					lastSeq = ev.SequenceNumber
				}
				_ = w.Flush()
			}
		}

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, err := w.WriteString("event: heartbeat\ndata: {}\n\n")
				if err != nil {
					return
				}
				_ = w.Flush()
			case msg, ok := <-eventChan:
				if !ok {
					return
				}
				var payload struct {
					SequenceNumber int    `json:"sequence_number"`
					EventType      string `json:"event_type"`
				}
				_ = json.Unmarshal([]byte(msg), &payload)
				if payload.EventType == "redis_reconnected" {
					if lastSeq > 0 {
						time.Sleep(time.Duration(mathrand.Int64N(500)) * time.Millisecond)
						metrics.GetCollector().IncrSSEReplayTotal()
						missedEvents, err := h.domainService.ListProjectEventsAfterSequence(uint(projectID), lastSeq)
						if err == nil {
							for _, ev := range missedEvents {
								evBytes, _ := json.Marshal(ev)
								_, _ = w.WriteString(fmt.Sprintf("id: %d\nevent: domain_event\ndata: %s\n\n", ev.SequenceNumber, evBytes))
								lastSeq = ev.SequenceNumber
							}
							_ = w.Flush()
						}
					}
					continue
				}
				if payload.EventType == "overflow" {
					metrics.GetCollector().IncrSSEOverflowTotal()
					_, _ = w.WriteString("event: overflow\ndata: {\"error\":\"overflow\",\"message\":\"Subscriber buffer overflow, forcing reconnect and replay\"}\n\n")
					_ = w.Flush()
					return
				}
				if payload.SequenceNumber > 0 {
					lastSeq = payload.SequenceNumber
					_, _ = w.WriteString(fmt.Sprintf("id: %d\n", payload.SequenceNumber))
				}
				_, err := w.WriteString(fmt.Sprintf("event: domain_event\ndata: %s\n\n", msg))
				if err != nil {
					return
				}
				_ = w.Flush()
			}
		}
	})

	return nil
}

func (h *DomainHandler) GetTraefikConfig(c *fiber.Ctx) error {
	// 1. Try to get cached config from Redis
	var cachedJSON string
	cacheKey := "traefik:dynamic_config"
	err := h.redisService.GetCache(cacheKey, &cachedJSON)
	if err == nil && cachedJSON != "" {
		c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
		return c.SendString(cachedJSON)
	}

	// 2. Cache miss: Query verified/routable custom domains from Postgres
	var domains []models.CustomDomain
	err = h.db.
		Preload("Project").
		Where("status IN (?)", []string{
			string(models.DomainStatusDNSVerified),
			string(models.DomainStatusSSLQueued),
			string(models.DomainStatusSSLProvisioning),
			string(models.DomainStatusSSLActive),
			string(models.DomainStatusActive),
			string(models.DomainStatusPropagationPending),
			string(models.DomainStatusDegraded),
			string(models.DomainStatusRenewalPending),
			string(models.DomainStatusRenewalFailed),
		}).
		Find(&domains).Error

	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "database_error", "details": err.Error()})
	}

	// 3. Group domains by ProjectID
	projectDomains := make(map[uint][]models.CustomDomain)
	for _, d := range domains {
		if d.Project.ID != 0 {
			projectDomains[d.ProjectID] = append(projectDomains[d.ProjectID], d)
		}
	}

	// 4. Build Traefik routing configuration response
	type TraefikRouter struct {
		Rule        string   `json:"rule"`
		Service     string   `json:"service"`
		EntryPoints []string `json:"entryPoints"`
		Priority    int      `json:"priority"`
		Middlewares []string `json:"middlewares,omitempty"`
	}

	type TraefikServer struct {
		URL string `json:"url"`
	}

	type TraefikHealthCheck struct {
		Path     string `json:"path"`
		Interval string `json:"interval"`
		Timeout  string `json:"timeout"`
	}

	type TraefikLoadBalancer struct {
		Servers     []TraefikServer     `json:"servers"`
		HealthCheck *TraefikHealthCheck `json:"healthCheck,omitempty"`
	}

	type TraefikService struct {
		LoadBalancer TraefikLoadBalancer `json:"loadBalancer"`
	}

	type TraefikHTTP struct {
		Routers     map[string]TraefikRouter     `json:"routers"`
		Middlewares map[string]interface{}        `json:"middlewares,omitempty"`
		Services    map[string]TraefikService    `json:"services"`
	}

	type TraefikConfigResponse struct {
		HTTP TraefikHTTP `json:"http"`
	}

	resp := TraefikConfigResponse{
		HTTP: TraefikHTTP{
			Routers:     make(map[string]TraefikRouter),
			Middlewares: make(map[string]interface{}),
			Services:    make(map[string]TraefikService),
		},
	}

	// Sanitize domain to prevent Traefik rule injection via backticks/pipes.
	sanitizeDomain := func(domain string) string {
		cleaned := strings.ReplaceAll(domain, "`", "")
		cleaned = strings.ReplaceAll(cleaned, "|", "")
		cleaned = strings.ReplaceAll(cleaned, "(", "")
		cleaned = strings.ReplaceAll(cleaned, ")", "")
		return strings.TrimSpace(cleaned)
	}

	for _, cds := range projectDomains {
		if len(cds) == 0 {
			continue
		}
		proj := cds[0].Project

		// Generate rules for all custom domains mapped to this project
		var rules []string
		for _, cd := range cds {
			safeDomain := sanitizeDomain(cd.Domain)
			if safeDomain == "" {
				continue
			}
			rules = append(rules, fmt.Sprintf("Host(`%s`)", safeDomain))
		}
		if len(rules) == 0 {
			continue
		}
		ruleStr := strings.Join(rules, " || ")

		internalPort := proj.GetInternalPort()

		routerName := fmt.Sprintf("project-%s-custom", proj.Subdomain)
		serviceName := fmt.Sprintf("project-%s-custom", proj.Subdomain)

		// Priority 300 > user-projects wildcard (200) to match custom domains first.
		resp.HTTP.Routers[routerName] = TraefikRouter{
			Rule:        ruleStr,
			Service:     serviceName,
			EntryPoints: []string{"web"},
			Priority:    300,
			Middlewares: []string{"security-headers@file"},
		}

		targetURL := fmt.Sprintf("http://project-%s:%s", proj.Subdomain, internalPort)
		if proj.Status == models.StatusSleeping {
			middlewareName := fmt.Sprintf("project-%s-wakeup-redirect", proj.Subdomain)
			resp.HTTP.Middlewares[middlewareName] = map[string]interface{}{
				"redirectRegex": map[string]interface{}{
					"regex":       "^(https?)://([^/]+)(.*)$",
					"replacement": fmt.Sprintf("http://%s/proxy/wakeup?subdomain=%s&redirect_url=${1}://${2}${3}", proj.GetFullDomain(h.cfg.ProjectDomain), proj.Subdomain),
				},
			}
			targetURL = "http://paas-backend:8080"

			router := resp.HTTP.Routers[routerName]
			router.Middlewares = []string{"security-headers@file", middlewareName}
			resp.HTTP.Routers[routerName] = router
		}

		resp.HTTP.Services[serviceName] = TraefikService{
			LoadBalancer: TraefikLoadBalancer{
				Servers: []TraefikServer{
					{
						URL: targetURL,
					},
				},
				HealthCheck: &TraefikHealthCheck{
					Path:     proj.GetHealthCheckPath(),
					Interval: "10s",
					Timeout:  "3s",
				},
			},
		}
	}

	// 5. Serialize to JSON, cache in Redis, and return response
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "serialization_error", "details": err.Error()})
	}

	cachedJSON = string(respBytes)
	// Cache it for 24 hours, but we will explicitly invalidate it upon domain updates
	_ = h.redisService.SetCache(cacheKey, cachedJSON, 24*time.Hour)

	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	return c.SendString(cachedJSON)
}
