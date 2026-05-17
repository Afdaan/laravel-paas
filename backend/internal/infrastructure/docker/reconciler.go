package docker

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/pkg/utils"
)

type ContainerClassification string

const (
	ClassificationActiveWeb     ContainerClassification = "active_web"
	ClassificationActiveWorker  ContainerClassification = "active_worker"
	ClassificationActiveRollout ContainerClassification = "active_rollout"
	ClassificationDangling      ContainerClassification = "dangling_legacy"
)

type ContainerInstance struct {
	ID        string
	Name      string
	Subdomain string
	Role      string
	CreatedAt time.Time
}

// ListProjectContainers retrieves all running and stopped containers associated with a specific project subdomain,
// extracting critical operational labels for classification.
func (s *DockerService) ListProjectContainers(subdomain string) ([]ContainerInstance, error) {
	output, err := utils.Run(30*time.Second, "docker", "ps", "-a", "--format", "{{.ID}}###{{.Names}}###{{.Label \"paas.project_subdomain\"}}###{{.Label \"paas.container_role\"}}###{{.Label \"paas.rollout_created_at\"}}")
	if err != nil {
		return nil, fmt.Errorf("failed to list docker containers for reconciliation: %w", err)
	}

	webPrefix := fmt.Sprintf("paas-project-%s-", subdomain)
	workerPrefix := fmt.Sprintf("paas-worker-%s-", subdomain)

	lines := strings.Split(strings.TrimSpace(output.Stdout), "\n")
	var containers []ContainerInstance

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "###")
		if len(parts) < 2 {
			continue
		}

		id := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		subdomainLabel := ""
		roleLabel := ""
		createdAtLabel := ""

		if len(parts) >= 3 {
			subdomainLabel = strings.TrimSpace(parts[2])
		}
		if len(parts) >= 4 {
			roleLabel = strings.TrimSpace(parts[3])
		}
		if len(parts) >= 5 {
			createdAtLabel = strings.TrimSpace(parts[4])
		}

		// Ensure container belongs strictly to this project subdomain
		if subdomainLabel != subdomain && !strings.HasPrefix(name, webPrefix) && !strings.HasPrefix(name, workerPrefix) {
			continue
		}

		var createdTime time.Time
		if createdAtLabel != "" {
			if ts, err := strconv.ParseInt(createdAtLabel, 10, 64); err == nil && ts > 0 {
				createdTime = time.Unix(ts, 0)
			}
		}
		if createdTime.IsZero() {
			if res, err := utils.Run(5*time.Second, "docker", "inspect", "--format", "{{.Created}}", id); err == nil {
				if t, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(res.Stdout)); parseErr == nil {
					createdTime = t
				}
			}
		}

		containers = append(containers, ContainerInstance{
			ID:        id,
			Name:      name,
			Subdomain: subdomain,
			Role:      roleLabel,
			CreatedAt: createdTime,
		})
	}

	return containers, nil
}

// ClassifyContainer categorizes a container instance against active orchestration state,
// allowing granular lifecycle control for active, rollout, and dangling instances.
func ClassifyContainer(c ContainerInstance, activeWebID string, activeWorkerID string, activeRolloutID string) ContainerClassification {
	if c.ID == activeWebID || c.Name == activeWebID || strings.HasPrefix(activeWebID, c.ID) || strings.HasPrefix(c.ID, activeWebID) {
		return ClassificationActiveWeb
	}
	if activeWorkerID != "" && (c.ID == activeWorkerID || c.Name == activeWorkerID || strings.HasPrefix(activeWorkerID, c.ID) || strings.HasPrefix(c.ID, activeWorkerID)) {
		return ClassificationActiveWorker
	}
	if activeRolloutID != "" && (c.ID == activeRolloutID || c.Name == activeRolloutID || strings.HasPrefix(activeRolloutID, c.ID) || strings.HasPrefix(c.ID, activeRolloutID)) {
		return ClassificationActiveRollout
	}
	return ClassificationDangling
}

// ReconcileContainers executes a multi-generation reconciliation loop across project containers,
// enforcing a safety grace period before evicting unassigned or dangling legacy instances.
func (s *DockerService) ReconcileContainers(subdomain string, activeWebID string, activeWorkerID *string, activeRolloutID *string) {
	workerIDStr := ""
	if activeWorkerID != nil {
		workerIDStr = *activeWorkerID
	}
	rolloutIDStr := ""
	if activeRolloutID != nil {
		rolloutIDStr = *activeRolloutID
	}

	containers, err := s.ListProjectContainers(subdomain)
	if err != nil {
		slog.Warn("Reconciliation loop aborted: unable to list containers", "subdomain", subdomain, "error", err)
		return
	}

	for _, c := range containers {
		classification := ClassifyContainer(c, activeWebID, workerIDStr, rolloutIDStr)

		// Retain all active web, worker, and in-flight zero-downtime rollout containers
		if classification != ClassificationDangling {
			continue
		}

		// Enforce a strict 10-minute grace period to prevent race conditions during canary/blue-green promotions
		if !c.CreatedAt.IsZero() && time.Since(c.CreatedAt) < 10*time.Minute {
			slog.Info("Retaining unassigned container due to active grace period",
				"subdomain", subdomain,
				"container", c.Name,
				"age", time.Since(c.CreatedAt).Round(time.Second))
			continue
		}

		slog.Info("Evicting dangling legacy project container instance",
			"subdomain", subdomain,
			"container", c.Name,
			"id", c.ID,
			"role", c.Role)

		_ = utils.RunSilent(30*time.Second, "docker", "stop", c.ID)
		_ = utils.RunSilent(30*time.Second, "docker", "rm", "-f", c.ID)
	}
}
