// ===========================================
// Worker Manager Service
// ===========================================
// Orchestrates standalone worker containers,
// handles concurrency scaling and zero-downtime draining.
// ===========================================
package worker

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/infrastructure"
	"github.com/laravel-paas/backend/internal/infrastructure/docker"
	"github.com/laravel-paas/backend/internal/models"
	"github.com/laravel-paas/backend/internal/services/setting"
)

// WorkerManager manages lifecycle of standalone worker containers
type WorkerManager struct {
	cfg            *config.Config
	dockerService  *docker.DockerService
	redisService   *infrastructure.RedisService
	settingService *setting.SettingService
	running        bool
	mu             sync.Mutex
	stopChan       chan struct{}
}

// NewWorkerManager creates a new WorkerManager
func NewWorkerManager(
	cfg *config.Config,
	dockerService *docker.DockerService,
	redisService *infrastructure.RedisService,
	settingService *setting.SettingService,
) *WorkerManager {
	return &WorkerManager{
		cfg:            cfg,
		dockerService:  dockerService,
		redisService:   redisService,
		settingService: settingService,
		running:        false,
		stopChan:       make(chan struct{}),
	}
}

// StartWatchdog starts the periodic container monitoring and scaling loop
func (m *WorkerManager) StartWatchdog() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	slog.Info("Worker manager: initializing standalone worker orchestration watchdog")

	go func() {
		// Initial check
		m.manageWorkers()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-m.stopChan:
				slog.Info("Worker manager: stopping watchdog")
				return
			case <-ticker.C:
				m.manageWorkers()
			}
		}
	}()
}

// Stop stops the watchdog
func (m *WorkerManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	m.running = false
	close(m.stopChan)
}

func (m *WorkerManager) manageWorkers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	maxStr := m.settingService.Get(models.SettingMaxConcurrent, models.DefaultMaxConcurrent)
	maxWorkers, err := strconv.Atoi(maxStr)
	if err != nil || maxWorkers < 1 {
		maxWorkers = 3
	}

	var targetVersion string
	if err := m.redisService.GetCache("worker:target_version", &targetVersion); err != nil || targetVersion == "" {
		targetVersion = "latest"
	}

	// Find existing worker containers
	output, err := exec.Command("docker", "ps", "-a", "--filter", "name=paas-worker-s", "--format", "{{.Names}}|{{.Label \"id.paas.worker.slot\"}}|{{.Label \"id.paas.worker.version\"}}|{{.State}}").Output()
	if err != nil {
		slog.Error("Worker manager: failed to list docker containers", "error", err)
		return
	}

	type workerInstance struct {
		name    string
		slot    int
		version string
		state   string
	}

	activeSlots := make(map[int][]workerInstance)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		name := parts[0]
		slotStr := parts[1]
		version := parts[2]
		state := parts[3]

		slot, _ := strconv.Atoi(slotStr)
		if slot > 0 {
			activeSlots[slot] = append(activeSlots[slot], workerInstance{
				name:    name,
				slot:    slot,
				version: version,
				state:   state,
			})
		}
	}

	// 1. Check slots 1 to maxWorkers
	for slot := 1; slot <= maxWorkers; slot++ {
		instances := activeSlots[slot]
		hasCorrectRunning := false

		for _, inst := range instances {
			if inst.version == targetVersion && inst.state == "running" {
				hasCorrectRunning = true
			} else if inst.version != targetVersion && inst.state == "running" && !strings.Contains(inst.name, "-draining-") {
				// Old version running, trigger drain
				drainName := fmt.Sprintf("paas-worker-s%d-draining-%d", slot, time.Now().Unix())
				slog.Warn("Worker manager: draining outdated worker", "oldName", inst.name, "drainName", drainName, "oldVer", inst.version, "newVer", targetVersion)
				_ = exec.Command("docker", "rename", inst.name, drainName).Run()
				// Send SIGTERM to trigger drain
				_ = exec.Command("docker", "kill", "-s", "SIGTERM", drainName).Run()
			} else if inst.state == "exited" || inst.state == "dead" {
				// Cleanup dead container
				slog.Debug("Worker manager: removing exited container", "name", inst.name)
				_ = exec.Command("docker", "rm", "-f", inst.name).Run()
			}
		}

		if !hasCorrectRunning {
			// Spawn new container for this slot
			containerName := fmt.Sprintf("paas-worker-s%d-%s", slot, strings.ReplaceAll(targetVersion, ".", "-"))
			slog.Info("Worker manager: spawning new worker container", "slot", slot, "name", containerName, "version", targetVersion)

			// Remove any colliding name just in case
			_ = exec.Command("docker", "rm", "-f", containerName).Run()

			projectsHostPath := m.cfg.HostProjectsPath
			if projectsHostPath == "" {
				projectsHostPath = "/opt/laravel-paas/storage/projects"
			}
			dataHostPath := m.cfg.HostDataPath
			if dataHostPath == "" {
				dataHostPath = "/opt/laravel-paas/storage/data"
			}
			dockerSock := m.cfg.DockerSocket
			if dockerSock == "" {
				dockerSock = "/var/run/docker.sock"
			}

			runArgs := []string{
				"run", "-d",
				"--name", containerName,
				"--network", models.NetworkName,
				"--restart", "unless-stopped",
				"--cpus", "2.0",
				"--memory", "3g",
				"--memory-swap", "4g",
				"--label", fmt.Sprintf("id.paas.worker.slot=%d", slot),
				"--label", fmt.Sprintf("id.paas.worker.version=%s", targetVersion),
				"-v", fmt.Sprintf("%s:/var/www/html/storage/app", projectsHostPath),
				"-v", fmt.Sprintf("%s:/app/storage/projects", projectsHostPath),
				"-v", fmt.Sprintf("%s:/app/data", dataHostPath),
				"-v", fmt.Sprintf("%s:%s", dockerSock, dockerSock),
				"-e", "PROJECTS_PATH=/app/storage/projects",
				"-e", fmt.Sprintf("REDIS_HOST=%s", m.cfg.RedisHost),
				"-e", fmt.Sprintf("REDIS_PORT=%s", m.cfg.RedisPort),
				"-e", fmt.Sprintf("REDIS_PASSWORD=%s", m.cfg.RedisPassword),
				"-e", fmt.Sprintf("MYSQL_HOST=%s", m.cfg.MYSQLHost),
				"-e", fmt.Sprintf("MYSQL_PORT=%s", m.cfg.MYSQLPort),
				"-e", fmt.Sprintf("MYSQL_USER=%s", m.cfg.MYSQLUser),
				"-e", fmt.Sprintf("MYSQL_PASSWORD=%s", m.cfg.MYSQLPassword),
				"-e", fmt.Sprintf("MYSQL_DATABASE=%s", m.cfg.MYSQLDatabase),
				fmt.Sprintf("paas-worker:%s", targetVersion),
			}

			if err := exec.Command("docker", runArgs...).Run(); err != nil {
				slog.Error("Worker manager: failed to start worker container", "slot", slot, "error", err)
			}
		}
	}

	// 2. Scale down slots > maxWorkers
	for slot, instances := range activeSlots {
		if slot > maxWorkers {
			for _, inst := range instances {
				if inst.state == "running" && !strings.Contains(inst.name, "-draining-") {
					drainName := fmt.Sprintf("paas-worker-s%d-draining-%d", slot, time.Now().Unix())
					slog.Warn("Worker manager: scaling down worker slot", "slot", slot, "name", inst.name)
					_ = exec.Command("docker", "rename", inst.name, drainName).Run()
					_ = exec.Command("docker", "kill", "-s", "SIGTERM", drainName).Run()
				} else if inst.state == "exited" || inst.state == "dead" {
					_ = exec.Command("docker", "rm", "-f", inst.name).Run()
				}
			}
		}
	}
}
