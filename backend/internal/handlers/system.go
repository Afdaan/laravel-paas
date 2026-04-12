package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
)

type SystemHandler struct {
	userService   *services.UserService
	dockerService *services.DockerService
}

func NewSystemHandler(userService *services.UserService, dockerService *services.DockerService) *SystemHandler {
	return &SystemHandler{
		userService:   userService,
		dockerService: dockerService,
	}
}

// GetStats returns system and docker stats
func (h *SystemHandler) GetStats(c *fiber.Ctx) error {
	type result struct {
		data interface{}
		err  error
	}

	systemChan := make(chan result, 1)
	containersChan := make(chan result, 1)
	imagesChan := make(chan result, 1)
	networksChan := make(chan result, 1)
	volumesChan := make(chan result, 1)

	go func() {
		stats, err := h.dockerService.GetSystemStats()
		systemChan <- result{stats, err}
	}()

	go func() {
		containers, err := h.dockerService.ListAllContainers()
		containersChan <- result{containers, err}
	}()

	go func() {
		images, err := h.dockerService.ListAllImages()
		imagesChan <- result{images, err}
	}()

	go func() {
		networks, err := h.dockerService.ListAllNetworks()
		networksChan <- result{networks, err}
	}()

	go func() {
		volumes, err := h.dockerService.ListAllVolumes()
		volumesChan <- result{volumes, err}
	}()

	rSystem := <-systemChan
	rContainers := <-containersChan
	rImages := <-imagesChan
	rNetworks := <-networksChan
	rVolumes := <-volumesChan

	if rSystem.err != nil {
		return rSystem.err
	}

	// For non-critical errors, we return empty slices
	containers := []models.DockerContainer{}
	if rContainers.err == nil {
		containers = rContainers.data.([]models.DockerContainer)
	}

	images := []models.DockerImage{}
	if rImages.err == nil {
		images = rImages.data.([]models.DockerImage)
	}

	networks := []models.DockerNetwork{}
	if rNetworks.err == nil {
		networks = rNetworks.data.([]models.DockerNetwork)
	}

	volumes := []models.DockerVolume{}
	if rVolumes.err == nil {
		volumes = rVolumes.data.([]models.DockerVolume)
	}

	return c.JSON(fiber.Map{
		"system":     rSystem.data,
		"containers": containers,
		"images":     images,
		"networks":   networks,
		"volumes":    volumes,
	})
}

// PruneSystem cleans up unused docker images/containers
func (h *SystemHandler) PruneSystem(c *fiber.Ctx) error {
	err := h.dockerService.PruneImages()
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"message": "System pruned successfully"})
}

// GetInitStatus checks if the system has been initialized with at least one admin
func (h *SystemHandler) GetInitStatus(c *fiber.Ctx) error {
	initialized, err := h.userService.IsInitialized()
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{
		"is_initialized": initialized,
	})
}

// InitializeSystem creates the first superadmin user
func (h *SystemHandler) InitializeSystem(c *fiber.Ctx) error {
	type InitRequest struct {
		Name     string `json:"name" validate:"required"`
		Email    string `json:"email" validate:"required,email"`
		Password string `json:"password" validate:"required,min=8"`
	}

	var req InitRequest
	if err := c.BodyParser(&req); err != nil {
		return err
	}

	// Validate input (basic check)
	if req.Name == "" || req.Email == "" || len(req.Password) < 8 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid input. Name, email, and password (min 8 chars) are required.",
		})
	}

	user, err := h.userService.InitializeSuperAdmin(req.Name, req.Email, req.Password)
	if err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"message": "System initialized successfully",
		"user":    user,
	})
}
