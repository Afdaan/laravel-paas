package domain

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/apperr"
	"github.com/laravel-paas/backend/internal/models"
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

// RegisterRoutes registers the domain endpoints to a router
func (h *DomainHandler) RegisterRoutes(r fiber.Router) {
	r.Get("", h.List)
	r.Post("", h.Add)
	r.Delete("/:domainID", h.Remove)
	r.Post("/:domainID/verify", h.Verify)
	r.Get("/:domainID/diagnostic", h.Diagnostic)
	r.Post("/:domainID/transfer", h.Transfer)
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

	// Trigger Nginx & Traefik Sync
	project, err := h.projectService.GetProjectByID(uint(projectID))
	if err == nil {
		h.projectService.SyncProjectNginx(project)
		h.projectService.RecreateProjectZeroDowntime(project)
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
		return c.JSON(fiber.Map{
			"error": fiber.Map{
				"code":    "VERIFICATION_FAILED",
				"message": err.Error(),
			},
			"domain": domainData,
		})
	}

	// Trigger Nginx & Traefik Sync upon successful verification
	// We MUST reload the project to get the updated custom domains status
	updatedProject, err := h.projectService.GetProjectByID(uint(projectID))
	if err == nil {
		h.projectService.SyncProjectNginx(updatedProject)
		h.projectService.RecreateProjectZeroDowntime(updatedProject)
	} else {
		h.projectService.SyncProjectNginx(project)
		h.projectService.RecreateProjectZeroDowntime(project)
	}

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

func (h *DomainHandler) Diagnostic(c *fiber.Ctx) error {
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

	// Find the domain in the project
	var domain models.CustomDomain
	found := false
	for _, d := range project.CustomDomains {
		if d.ID == uint(domainID) {
			domain = d
			found = true
			break
		}
	}

	if !found {
		return apperr.New(404, "DOMAIN_NOT_FOUND", "Domain not found in this project")
	}

	diagnostic, err := h.domainService.GetDomainDiagnostic(domain.Domain, project)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"data": diagnostic,
	})
}
