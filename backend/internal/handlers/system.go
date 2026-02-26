package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services"
)

type SystemHandler struct {
	dockerService *services.DockerService
}

func NewSystemHandler(dockerService *services.DockerService) *SystemHandler {
	return &SystemHandler{dockerService: dockerService}
}

// GetStats returns system and docker stats
func (h *SystemHandler) GetStats(c *fiber.Ctx) error {
	stats, err := h.dockerService.GetSystemStats()
	if err != nil {
		return err
	}

	containers, err := h.dockerService.ListAllContainers()
	if err != nil {
		containers = []models.DockerContainer{}
	}

	images, err := h.dockerService.ListAllImages()
	if err != nil {
		images = []models.DockerImage{}
	}

	networks, err := h.dockerService.ListAllNetworks()
	if err != nil {
		networks = []models.DockerNetwork{}
	}

	volumes, err := h.dockerService.ListAllVolumes()
	if err != nil {
		volumes = []models.DockerVolume{}
	}

	return c.JSON(fiber.Map{
		"system":     stats,
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
