package docker

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

// ===========================================
// Logs & Stats
// ===========================================
// GetLogs retrieves logs from a specific container ID
func (s *DockerService) GetLogs(containerID string, lines int) (string, error) {
	res, err := utils.Run(15*time.Second, "docker", "logs", "--tail", strconv.Itoa(lines), containerID)
	if err != nil {
		return "", fmt.Errorf("failed to get logs for %s: %s", containerID, res.Stderr)
	}

	return res.Stdout + res.Stderr, nil
}

// ContainerStats represents resource usage
type ContainerStats struct {
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	MemoryMax  float64 `json:"memory_max_mb"`
}

// DockerStatsJSON represents the raw JSON output from docker stats
type DockerStatsJSON struct {
	CPUPerc  string `json:"CPUPerc"`
	MemUsage string `json:"MemUsage"`
}

// GetContainerStats retrieves container resource usage
func (s *DockerService) GetContainerStats(containerID string) (*ContainerStats, error) {
	res, err := utils.Run(10*time.Second, "docker", "stats", "--no-stream", "--format", "{{json .}}", containerID)

	if err != nil {
		slog.Error("Docker stats request failed", "containerId", containerID, "error", err, "stderr", res.Stderr)
		return nil, fmt.Errorf("docker stats failed: %s", res.Stderr)
	}

	// Output might contain multiple lines if multiple containers match (unlikely here)
	// or just one JSON object.
	output := strings.TrimSpace(res.Stdout)

	if output == "" {
		slog.Warn("Docker stats returned empty output", "containerId", containerID)
		return nil, fmt.Errorf("container not found or not running")
	}

	var rawStats DockerStatsJSON
	if err := json.Unmarshal([]byte(output), &rawStats); err != nil {
		slog.Error("Failed to unmarshal docker stats", "containerId", containerID, "output", output, "error", err)
		return nil, fmt.Errorf("failed to parse docker stats: %v", err)
	}

	stats := &ContainerStats{}

	// 1. Parse CPU (remove % and trim)
	cpuStr := strings.ReplaceAll(rawStats.CPUPerc, "%", "")
	cpuVal, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64)
	if err != nil {
		slog.Warn("Failed to parse CPU percentage from docker stats", "cpuStr", cpuStr, "error", err)
		stats.CPUPercent = 0
	} else {
		stats.CPUPercent = cpuVal
	}

	// 2. Parse Memory (format: USAGE / LIMIT)
	// Example: "12.5MiB / 1.94GiB"
	parts := strings.Split(rawStats.MemUsage, "/")
	if len(parts) >= 2 {
		stats.MemoryMB = parseMemoryBytes(strings.TrimSpace(parts[0]))
		stats.MemoryMax = parseMemoryBytes(strings.TrimSpace(parts[1]))
	} else {
		slog.Warn("Unexpected memory format in docker stats", "memUsage", rawStats.MemUsage)
	}

	slog.Debug("Parsed container stats", "containerId", containerID, "cpu", stats.CPUPercent, "memoryMB", stats.MemoryMB)

	return stats, nil
}

// GetAllContainerStats retrieves resource usage for all containers
func (s *DockerService) GetAllContainerStats() (map[string]ContainerStats, error) {
	res, err := utils.Run(15*time.Second, "docker", "stats", "--no-stream", "--format", "{{.ID}}|{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}")

	if err != nil {
		slog.Error("Global docker stats request failed", "error", err, "stderr", res.Stderr)
		return nil, fmt.Errorf("docker stats failed: %s", res.Stderr)
	}

	result := make(map[string]ContainerStats)
	output := res.Stdout
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		containerID := parts[0]
		// containerName := parts[1]
		cpuPerc := parts[2]
		memUsage := parts[3]

		stats := ContainerStats{}

		// Parse CPU
		cpuStr := strings.ReplaceAll(cpuPerc, "%", "")
		if val, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64); err == nil {
			stats.CPUPercent = val
		}

		// Parse Memory (Usage / Limit)
		memParts := strings.Split(memUsage, "/")
		if len(memParts) >= 2 {
			stats.MemoryMB = parseMemoryBytes(strings.TrimSpace(memParts[0]))
			stats.MemoryMax = parseMemoryBytes(strings.TrimSpace(memParts[1]))
		}

		result[containerID] = stats
	}

	return result, nil
}

// ===========================================
// System & Global Docker Info
// ===========================================
// GetSystemStats retrieves host machine resource usage with robust detection
func (s *DockerService) GetSystemStats() (*models.SystemStats, error) {
	stats := &models.SystemStats{
		DiskPath: s.cfg.ProjectsPath,
		OS:       "Linux",
		CPUCores: 1,
	}

	// 1. Hostname
	if hostname, err := os.Hostname(); err == nil {
		stats.Hostname = hostname
	}

	// 2. OS Distribution Detection
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				stats.OS = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}

	// 3. Docker Mode Detection
	if _, err := os.Stat("/.dockerenv"); err == nil {
		stats.IsDocker = true
	} else if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		if strings.Contains(string(data), "docker") || strings.Contains(string(data), "containerd") {
			stats.IsDocker = true
		}
	}

	// 4. CPU Usage (Using /proc/stat - two samples for accuracy)
	readCpuStat := func() (uint64, uint64) {
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, 0
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "cpu ") {
				fields := strings.Fields(line)
				if len(fields) < 5 {
					return 0, 0
				}
				user, _ := strconv.ParseUint(fields[1], 10, 64)
				nice, _ := strconv.ParseUint(fields[2], 10, 64)
				system, _ := strconv.ParseUint(fields[3], 10, 64)
				idle, _ := strconv.ParseUint(fields[4], 10, 64)
				iowait, _ := strconv.ParseUint(fields[5], 10, 64)
				irq, _ := strconv.ParseUint(fields[6], 10, 64)
				softirq, _ := strconv.ParseUint(fields[7], 10, 64)
				total := user + nice + system + idle + iowait + irq + softirq
				return total, idle
			}
		}
		return 0, 0
	}

	t1, i1 := readCpuStat()
	time.Sleep(200 * time.Millisecond)
	t2, i2 := readCpuStat()

	if t2 > t1 {
		totalDiff := float64(t2 - t1)
		idleDiff := float64(i2 - i1)
		stats.CPUUsage = (totalDiff - idleDiff) * 100.0 / totalDiff
	}

	// 5. CPU Cores
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		stats.CPUCores = strings.Count(string(data), "processor")
	}

	// 6. Memory Usage (Using /proc/meminfo)
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var total, available, free, buffers, cached uint64
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			key := strings.TrimSuffix(fields[0], ":")
			val, _ := strconv.ParseUint(fields[1], 10, 64)
			switch key {
			case "MemTotal":
				total = val * 1024
			case "MemFree":
				free = val * 1024
			case "MemAvailable":
				available = val * 1024
			case "Buffers":
				buffers = val * 1024
			case "Cached":
				cached = val * 1024
			}
		}
		if total > 0 {
			if available == 0 {
				available = free + buffers + cached
			}
			stats.MemoryTotal = total
			stats.MemoryUsed = total - available
		}
	}

	// 7. Disk Usage
	if res, err := utils.Run(5*time.Second, "df", "-b", s.cfg.ProjectsPath); err == nil {
		lines := strings.Split(res.Stdout, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 3 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				used, _ := strconv.ParseUint(fields[2], 10, 64)
				stats.DiskTotal = total
				stats.DiskUsed = used
			}
		}
	}

	// 8. Docker Version
	if res, err := utils.Run(5*time.Second, "docker", "version", "--format", "{{.Server.Version}}"); err == nil {
		stats.DockerVersion = strings.TrimSpace(res.Stdout)
	}

	return stats, nil
}

func (s *DockerService) ListAllContainers() ([]models.DockerContainer, error) {
	// 1. Get stats for info merging in parallel (or just call it once as it's already one call)
	statsMap, _ := s.GetAllContainerStats()

	// 2. Get all container IDs first
	cmdIDs := exec.Command("docker", "ps", "-a", "-q")
	idsOut, err := cmdIDs.Output()
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(idsOut))
	if len(ids) == 0 {
		return []models.DockerContainer{}, nil
	}

	// 3. Batch inspect all containers
	// Format: ID|Names|Image|State|Status|CreatedAt|IPAddress|Ports
	// Note: We use .State.Status for State, and formatted Status string for Status
	// We'll get IP and Ports in a single call
	format := "{{.Id}}|{{.Name}}|{{.Config.Image}}|{{.State.Status}}|{{.State.Status}}|{{.Created}}|{{range .NetworkSettings.Networks}}{{.IPAddress}},{{end}}|{{range $p, $conf := .NetworkSettings.Ports}}{{range $conf}}{{.HostIp}}:{{.HostPort}}->{{$p}},{{end}}{{end}}"
	args := append([]string{"inspect", "--format", format}, ids...)
	cmdInspect := exec.Command("docker", args...)
	output, err := cmdInspect.Output()
	if err != nil {
		return nil, err
	}

	var result []models.DockerContainer
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 8 {
			continue
		}

		id := parts[0]
		// docker inspect names start with /
		name := strings.TrimPrefix(parts[1], "/")

		// Parse creation time (Docker uses ISO8601 for inspect)
		created, _ := time.Parse(time.RFC3339Nano, parts[5])

		container := models.DockerContainer{
			ID:        id[:12], // Shorten ID for consistency with ps
			Names:     []string{name},
			Image:     parts[2],
			State:     parts[3],
			Status:    parts[4],
			CreatedAt: created,
			IPAddress: strings.TrimSuffix(parts[6], ","),
			Ports:     parsePorts(strings.TrimSuffix(parts[7], ",")),
		}

		// Merge stats if available
		shortID := id[:12]
		if stats, ok := statsMap[shortID]; ok {
			container.CPUPercent = stats.CPUPercent
			container.MemoryUsage = stats.MemoryMB
		} else if stats, ok := statsMap[id]; ok {
			container.CPUPercent = stats.CPUPercent
			container.MemoryUsage = stats.MemoryMB
		} else {
			// Try by name
			if stats, ok := statsMap[name]; ok {
				container.CPUPercent = stats.CPUPercent
				container.MemoryUsage = stats.MemoryMB
			}
		}

		result = append(result, container)
	}

	return result, nil
}

// parsePorts converts docker port string to slice
func parsePorts(portStr string) []string {
	if portStr == "" {
		return []string{}
	}
	// Example: "0.0.0.0:80->80/tcp, :::80->80/tcp" or "80/tcp"
	return strings.Split(portStr, ",")
}

// ListAllImages returns all images on the host
func (s *DockerService) ListAllImages() ([]models.DockerImage, error) {
	// Format: ID|Repo|Tag|Size|CreatedAt
	res, err := utils.Run(10*time.Second, "docker", "images", "--format", "{{.ID}}|{{.Repository}}|{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
	if err != nil {
		return nil, err
	}

	// Get used images to mark status
	usedImages := make(map[string]bool)
	cmdUsed := exec.Command("docker", "ps", "-a", "--format", "{{.Image}}")
	outUsed, _ := cmdUsed.Output()
	for _, img := range strings.Split(string(outUsed), "\n") {
		img = strings.TrimSpace(img)
		if img != "" {
			usedImages[img] = true
		}
	}

	var result []models.DockerImage
	lines := strings.Split(string(res.Stdout), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}

		repo := parts[1]
		tag := parts[2]

		status := "Unused"
		// Match against full name or ID
		if usedImages[repo] || usedImages[repo+":"+tag] || usedImages[parts[0]] {
			status = "In Use"
		}

		result = append(result, models.DockerImage{
			ID:         parts[0],
			Repository: repo,
			Tag:        tag,
			SizeHuman:  parts[3],
			Status:     status,
		})
	}

	return result, nil
}

// ListAllNetworks returns all networks on the host
func (s *DockerService) ListAllNetworks() ([]models.DockerNetwork, error) {
	// Format: ID|Name|Driver|Scope
	res, err := utils.Run(10*time.Second, "docker", "network", "ls", "--format", "{{.ID}}|{{.Name}}|{{.Driver}}|{{.Scope}}")
	if err != nil {
		return nil, err
	}

	// Get used networks
	usedNets := make(map[string]bool)
	cmdUsed := exec.Command("docker", "ps", "-a", "--format", "{{.Networks}}")
	outUsed, _ := cmdUsed.Output()
	for _, netLine := range strings.Split(string(outUsed), "\n") {
		for _, net := range strings.Split(netLine, ",") {
			usedNets[strings.TrimSpace(net)] = true
		}
	}

	var result []models.DockerNetwork
	lines := strings.Split(string(res.Stdout), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		name := parts[1]
		status := "Unused"
		if usedNets[name] {
			status = "In Use"
		}

		result = append(result, models.DockerNetwork{
			ID:     parts[0],
			Name:   name,
			Driver: parts[2],
			Scope:  parts[3],
			Status: status,
		})
	}

	return result, nil
}

// ListAllVolumes returns all volumes on the host
func (s *DockerService) ListAllVolumes() ([]models.DockerVolume, error) {
	// Format: Name|Driver|Mountpoint
	res, err := utils.Run(10*time.Second, "docker", "volume", "ls", "--format", "{{.Name}}|{{.Driver}}|{{.Mountpoint}}")
	if err != nil {
		return nil, err
	}

	var result []models.DockerVolume
	lines := strings.Split(string(res.Stdout), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}

		result = append(result, models.DockerVolume{
			Name:       parts[0],
			Driver:     parts[1],
			Mountpoint: parts[2],
			Status:     "Active", // Volume status is harder to determine simply
		})
	}

	return result, nil
}

// Convert docker memory headers (GiB, MiB, kiB, B) to MB
func parseMemoryBytes(memStr string) float64 {
	input := strings.TrimSpace(memStr)
	valueStr := ""
	unit := ""
	for i, r := range input {
		if (r < '0' || r > '9') && r != '.' {
			valueStr = input[:i]
			unit = strings.TrimSpace(input[i:])
			break
		}
	}
	if valueStr == "" {
		return 0
	}
	val, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0
	}
	switch strings.ToLower(unit) {
	case "gib", "gb":
		return val * 1024
	case "mib", "mb":
		return val
	case "kib", "kb":
		return val / 1024
	case "b":
		return val / 1024 / 1024
	default:
		return val
	}
}

// ExecProjectCommand runs commands inside container (artisan for Laravel, direct binary execution for others)
// Tokenized arguments bypass intermediate shells (sh -c) to eliminate command injection vulnerabilities.
func (s *DockerService) ExecProjectCommand(project *models.Project, command string) (string, error) {
	containerID := ""
	if project.ContainerID != nil {
		containerID = *project.ContainerID
	}
	if containerID == "" {
		return "", fmt.Errorf("container not running")
	}

	// Determine the best user to run the command
	// For Laravel (legacy) it's usually www-data
	// For Node.js (railpack) it's often 'node' (UID 1000) or root
	user := "root"
	potentialUsers := []string{"www-data", "node"}
	for _, u := range potentialUsers {
		if res, err := utils.Run(5*time.Second, "docker", "exec", containerID, "id", "-u", u); err == nil && strings.TrimSpace(res.Stdout) != "" {
			user = u
			break
		}
	}

	parser := utils.NewSecureCommandParser(true)

	// Prepare execution args based on framework
	var fullArgs []string
	if project.Framework == "Laravel" {
		cmdStr := parseArtisanMigrationCommand(command)
		tokens, err := parser.Tokenize(cmdStr)
		if err != nil {
			return "", fmt.Errorf("command security validation failed: %w", err)
		}

		// Fix permissions as root before running as www-data to avoid "Permission denied" errors in logs/cache
		if user != "root" {
			if res, err := utils.Run(10*time.Second, "docker", "exec", "-u", "root", containerID, "chown", "-R", user+":"+user, "storage", "bootstrap/cache"); err != nil {
				slog.Warn("Failed to fix permissions before command execution", "error", err, "stderr", res.Stderr)
			}
		}

		fullArgs = []string{"exec", "-u", user, containerID, "php", "artisan"}
		fullArgs = append(fullArgs, tokens...)
	} else {
		// For other frameworks (Node.js, Go, etc.), tokenize and run command directly
		tokens, err := parser.Tokenize(command)
		if err != nil {
			return "", fmt.Errorf("command security validation failed: %w", err)
		}
		fullArgs = []string{"exec", "-u", user, containerID}
		fullArgs = append(fullArgs, tokens...)
	}

	res, err := utils.Run(2*time.Minute, "docker", fullArgs...)

	output := strings.TrimSpace(res.Stdout + "\n" + res.Stderr)
	if err != nil {
		if output == "" {
			output = "Command failed with no output"
		}
		return output, fmt.Errorf("command failed: %w", err)
	}

	return output, nil
}

// parseArtisanMigrationCommand deterministically inspects an artisan command string.
// If the command is an operational database migration or seed command, it enforces non-interactive and force execution flags.
func parseArtisanMigrationCommand(cmdStr string) string {
	fields := strings.Fields(cmdStr)
	if len(fields) == 0 {
		return cmdStr
	}

	cmdName := fields[0]
	isMigration := false
	switch cmdName {
	case "migrate", "migrate:fresh", "migrate:refresh", "migrate:rollback", "migrate:reset", "db:seed":
		isMigration = true
	}

	if !isMigration {
		return cmdStr
	}

	result := cmdStr
	if !strings.Contains(result, "--no-interaction") && !strings.Contains(result, "-n") {
		result += " --no-interaction"
	}
	if !strings.Contains(result, "--force") {
		result += " --force"
	}
	return result
}

// RemoveImage removes a project's docker image
func (s *DockerService) RemoveImage(subdomain string) error {
	imageName := fmt.Sprintf("paas-%s", subdomain)
	if err := exec.Command("docker", "rmi", imageName).Run(); err != nil {
		slog.Warn("Failed to remove image", "image", imageName, "error", err)
	}
	return nil
}

// PruneImages removes dangling images and unused project images
func (s *DockerService) PruneImages() error {
	slog.Info("Starting Docker image pruning")

	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f"); err != nil {
		slog.Warn("Failed to prune dangling images", "error", err)
	}

	filter := fmt.Sprintf("label=%s", models.LabelProjectManaged)
	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f", "--filter", filter); err != nil {
		slog.Warn("Failed to prune project images", "error", err)
	}

	return nil
}

// CleanupProject removes project files
func (s *DockerService) CleanupProject(subdomain string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.RemoveAll(projectPath)
}



