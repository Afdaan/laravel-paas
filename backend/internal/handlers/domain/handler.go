package domain

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/services/domain"
	projectServicePkg "github.com/laravel-paas/backend/internal/services/project"
)

type DomainHandler struct {
	domainService  *domain.DomainService
	projectService *projectServicePkg.ProjectService
}

func NewDomainHandler(domainService *domain.DomainService, projectService *projectServicePkg.ProjectService) *DomainHandler {
	return &DomainHandler{
		domainService:  domainService,
		projectService: projectService,
	}
}

// Routes returns the router for domain endpoints
func (h *DomainHandler) Routes() *fiber.App {
	r := fiber.New()

	r.Get("/", h.List)
	r.Get("/all", h.ListAll)
	r.Post("/", h.Add)
	r.Delete("/:domainID", h.Remove)
	r.Post("/:domainID/verify", h.Verify)
	r.Post("/:domainID/transfer", h.Transfer)

	return r
}

func (h *DomainHandler) List(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
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

func (h *DomainHandler) Add(c *fiber.Ctx) error {
	projectID, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_ID", "Invalid project ID")
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

	if err := h.domainService.RemoveDomain(uint(domainID), uint(projectID)); err != nil {
		return err
	}

	// Trigger Nginx Sync
	project, err := h.projectService.GetProjectByID(uint(projectID))
	if err == nil {
		h.projectService.SyncProjectNginx(project)
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

	project, err := h.projectService.GetProjectByID(uint(projectID))
	if err != nil {
		return apperr.New(404, "PROJECT_NOT_FOUND", "Project not found")
	}

	domainData, err := h.domainService.VerifyDomain(uint(domainID), uint(projectID), project)
	if err != nil {
		// Even if error, we return the domain so the frontend knows the status
		return c.Status(400).JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "VERIFICATION_FAILED",
				"message": err.Error(),
			},
			"domain": domainData,
		})
	}

	// Trigger Nginx Sync upon successful verification
	h.projectService.SyncProjectNginx(project)

	return c.JSON(fiber.Map{
		"data": domainData,
	})
}

func (h *DomainHandler) Transfer(c *fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)
	domainID, err := strconv.ParseUint(c.Params("domainID"), 10, 32)
	if err != nil {
		return apperr.New(400, "INVALID_DOMAIN_ID", "Invalid domain ID")
	}

	var req struct {
		TargetProjectID uint `json:"target_project_id"`
	}

	if err := c.BodyParser(&req); err != nil {
		return apperr.New(400, "INVALID_REQUEST", "Invalid request body")
	}

	if err := h.domainService.TransferDomain(userID, uint(domainID), req.TargetProjectID); err != nil {
		return err
	}

	return c.JSON(fiber.Map{"message": "Domain transferred successfully"})
}
