package domain

import (
	"bufio"
	"encoding/json"
	"fmt"
	mathrand "math/rand/v2"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/metrics"
	"github.com/laravel-paas/shared/services/domain"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
)

type DomainHandler struct {
	cfg            *config.Config
	domainService  *domain.DomainService
	projectService *projectServicePkg.ProjectService
}

func NewDomainHandler(cfg *config.Config, domainService *domain.DomainService, projectService *projectServicePkg.ProjectService) *DomainHandler {
	return &DomainHandler{
		cfg:            cfg,
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

	// Trigger Nginx & Traefik Sync
	if project != nil {
		_, _ = h.projectService.SyncProjectNginxFrom(project, "domain_remove_handler")
		_ = h.projectService.RestartProject(project)
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
			_ = h.projectService.RestartProject(updatedProject)
		} else {
			_, _ = h.projectService.SyncProjectNginxFrom(project, "domain_verify_handler_fallback")
			_ = h.projectService.RestartProject(project)
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
