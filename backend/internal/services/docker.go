// ===========================================
// Docker Service
// ===========================================
// Manages Docker containers for student projects
// ===========================================
package services

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/config"
	"github.com/laravel-paas/backend/internal/models"
)

// DockerService handles all Docker operations
type DockerService struct {
	cfg *config.Config
}

// NewDockerService creates a new Docker service
func NewDockerService(cfg *config.Config) *DockerService {
	return &DockerService{cfg: cfg}
}

// ===========================================
// Repository Operations
// ===========================================

// CloneRepository clones a GitHub repository
func (s *DockerService) CloneRepository(githubURL, branch, subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)

	// Check if .env exists and backup its content
	var envBackup []byte
	envPath := filepath.Join(projectPath, ".env")
	if data, err := os.ReadFile(envPath); err == nil {
		envBackup = data
	}

	// Remove existing directory if present
	os.RemoveAll(projectPath)

	// Clone specific branch
	args := []string{"clone", "--depth=1", "-b", branch, githubURL, projectPath}
	cmd := exec.Command("git", args...)
	
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone failed: %s", stderr.String())
	}

	// Restore .env if backup exists
	if envBackup != nil {
		if err := os.WriteFile(envPath, envBackup, 0644); err != nil {
			fmt.Printf("Warning: Failed to restore .env file: %v\n", err)
		}
	}

	// Verify it's a Laravel project
	if _, err := os.Stat(filepath.Join(projectPath, "artisan")); os.IsNotExist(err) {
		os.RemoveAll(projectPath)
		return "", fmt.Errorf("not a valid Laravel project (missing artisan file)")
	}

	return projectPath, nil
}

// ===========================================
// Version Detection
// ===========================================

// ComposerJSON represents the structure of composer.json
type ComposerJSON struct {
	Require map[string]string `json:"require"`
}

// DetectVersions reads composer.json to detect Laravel and PHP versions
func (s *DockerService) DetectVersions(projectPath string) (laravelVersion, phpVersion string, err error) {
	composerPath := filepath.Join(projectPath, "composer.json")

	data, err := os.ReadFile(composerPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to read composer.json: %w", err)
	}

	var composer ComposerJSON
	if err := json.Unmarshal(data, &composer); err != nil {
		return "", "", fmt.Errorf("failed to parse composer.json: %w", err)
	}

	// Detect Laravel version
	laravelReq := composer.Require["laravel/framework"]
	laravelVersion = extractMajorVersion(laravelReq)

	// Detect PHP version
	phpReq := composer.Require["php"]
	phpVersion = detectPHPVersion(laravelVersion, phpReq)

	return laravelVersion, phpVersion, nil
}

// extractMajorVersion extracts major version from version constraint
func extractMajorVersion(constraint string) string {
	// Match patterns like ^11.0, ~11.0, 11.*, >=11.0
	re := regexp.MustCompile(`(\d+)\.`)
	matches := re.FindStringSubmatch(constraint)
	if len(matches) > 1 {
		return matches[1]
	}
	return "11" // Default to Laravel 11
}

// detectPHPVersion determines the appropriate PHP version
func detectPHPVersion(laravelVersion, phpConstraint string) string {
	// Map Laravel versions to minimum PHP versions
	laravelPHPMap := map[string]string{
		"8":  "8.0",
		"9":  "8.1",
		"10": "8.2",
		"11": "8.3",
	}

	// Use Laravel version mapping if available
	if php, ok := laravelPHPMap[laravelVersion]; ok {
		return php
	}

	// Try to extract from PHP constraint
	re := regexp.MustCompile(`(\d+\.\d+)`)
	matches := re.FindStringSubmatch(phpConstraint)
	if len(matches) > 1 {
		return matches[1]
	}

	return "8.3" // Default to PHP 8.3
}

// ===========================================
// Database Operations
// ===========================================

// CreateDatabase creates a MySQL database for a project
func (s *DockerService) CreateDatabase(dbName string) error {
	// Connect to MySQL container and create database
	cmd := exec.Command("docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+os.Getenv("MYSQL_ROOT_PASSWORD"),
		"-e", fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", dbName))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create database: %s", stderr.String())
	}

	// Create user and grant privileges
	cmd = exec.Command("docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+os.Getenv("MYSQL_ROOT_PASSWORD"),
		"-e", fmt.Sprintf(
			"CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'; GRANT ALL PRIVILEGES ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;",
			dbName, dbName, dbName, dbName,
		))

	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create database user: %s", stderr.String())
	}

	return nil
}

// DropDatabase removes a MySQL database
func (s *DockerService) DropDatabase(dbName string) error {
	cmd := exec.Command("docker", "exec", "paas-mysql",
		"mysql", "-uroot", "-p"+os.Getenv("MYSQL_ROOT_PASSWORD"),
		"-e", fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; DROP USER IF EXISTS '%s'@'%%';", dbName, dbName))

	cmd.Run() // Ignore errors
	return nil
}

// ===========================================
// Container Operations
// ===========================================

// BuildAndRun builds and starts a container for a project
func (s *DockerService) BuildAndRun(project *models.Project, phpVersion, projectDomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, project.Subdomain)

	// Copy appropriate Dockerfile
	dockerfile := fmt.Sprintf("Dockerfile.php%s", strings.ReplaceAll(phpVersion, ".", ""))
	srcDockerfile := filepath.Join(s.cfg.TemplatesPath, dockerfile)
	dstDockerfile := filepath.Join(projectPath, "Dockerfile")

	if err := copyFile(srcDockerfile, dstDockerfile); err != nil {
		return "", fmt.Errorf("failed to copy Dockerfile: %w", err)
	}

	// Copy nginx and supervisor configs
	if err := copyFile(filepath.Join(s.cfg.TemplatesPath, "nginx.conf"),
		filepath.Join(projectPath, "docker", "nginx.conf")); err != nil {
		// Create docker directory if needed
		os.MkdirAll(filepath.Join(projectPath, "docker"), 0755)
		copyFile(filepath.Join(s.cfg.TemplatesPath, "nginx.conf"),
			filepath.Join(projectPath, "docker", "nginx.conf"))
	}
	copyFile(filepath.Join(s.cfg.TemplatesPath, "supervisord.conf"),
		filepath.Join(projectPath, "docker", "supervisord.conf"))

	// Append Config for Queue Worker if enabled
	if project.QueueEnabled {
		workerConfig := `
[program:laravel-worker]
process_name=%(program_name)s_%(process_num)02d
command=/bin/sh -c "sleep 20 && php /var/www/html/artisan queue:work database --sleep=3 --tries=3"
autostart=true
autorestart=true
user=www-data
numprocs=1
redirect_stderr=true
stdout_logfile=/dev/stdout
stdout_logfile_maxbytes=0
`
		f, err := os.OpenFile(filepath.Join(projectPath, "docker", "supervisord.conf"), os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return "", fmt.Errorf("failed to open supervisor config: %w", err)
		}
		defer f.Close()
		
		if _, err := f.WriteString(workerConfig); err != nil {
			return "", fmt.Errorf("failed to append supervisor config: %w", err)
		}
	}

	// Create .env file for Laravel
	if err := s.createEnvFile(project, projectPath, projectDomain); err != nil {
		return "", fmt.Errorf("failed to create .env: %w", err)
	}

	// Build image
	imageName := fmt.Sprintf("paas-%s", project.Subdomain)

	var stdout, stderr bytes.Buffer

	buildArgs := []string{"buildx", "build", "--load", 
		"--label", "com.paas.project=true",
		"-t", imageName, projectPath}
	cmd := exec.Command("docker", buildArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Fallback to classic build for environments without buildx
		stdout.Reset()
		stderr.Reset()

		cmd = exec.Command("docker", "build", 
			"--label", "com.paas.project=true",
			"-t", imageName, projectPath)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err2 := cmd.Run(); err2 != nil {
			return "", fmt.Errorf("docker build failed: %s%s", stdout.String(), stderr.String())
		}
	}

	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)
	
	// Blue-green deployment: unique router per deployment, shared service for traffic switching
	routerName := fmt.Sprintf("%s-%d", project.Subdomain, timestamp)
	serviceName := project.Subdomain

	runArgs := []string{
		"run", "-d",
		"--name", containerName,
		"--network", s.cfg.DockerNetwork,
		"--restart", "unless-stopped",
		"--cpus", "0.5",
		"--memory", "512m",
		
		"--label", "traefik.enable=true",
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)",
			routerName, project.Subdomain, projectDomain),
		"--label", fmt.Sprintf("traefik.http.routers.%s.service=%s", routerName, serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port=80", serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.path=/health", serviceName),
		"--label", fmt.Sprintf("traefik.http.services.%s.loadbalancer.healthcheck.interval=2s", serviceName),
		
		imageName,
	}

	cmd = exec.Command("docker", runArgs...)
	stdout.Reset()
	stderr.Reset()
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker run failed: %s", stderr.String())
	}

	containerID := strings.TrimSpace(stdout.String())

	// Run migrations
	go func() {
		time.Sleep(10 * time.Second) // Wait for container to start
		exec.Command("docker", "exec", containerName,
			"php", "artisan", "migrate", "--force").Run()
	}()

	return containerID, nil
}

// createEnvFile generates .env for Laravel project
func (s *DockerService) createEnvFile(project *models.Project, projectPath, projectDomain string) error {
	// Generate random 32-byte key for APP_KEY
	key := make([]byte, 32)
	rand.Read(key)
	appKey := base64.StdEncoding.EncodeToString(key)

	envContent := fmt.Sprintf(`APP_NAME="%s"
APP_ENV=production
APP_KEY=base64:%s
APP_DEBUG=true
APP_URL=https://%s.%s

DB_CONNECTION=mysql
DB_HOST=paas-mysql
DB_PORT=3306
DB_DATABASE=%s
DB_USERNAME=%s
DB_PASSWORD=%s

CACHE_DRIVER=file
SESSION_DRIVER=file
QUEUE_CONNECTION=sync
`,
		project.Name,
		appKey,
		project.Subdomain, projectDomain,
		project.DatabaseName,
		project.DatabaseName,
		project.DatabaseName,
	)
	
	// Set QUEUE_CONNECTION dynamically
	queueConn := "sync"
	if project.QueueEnabled {
		queueConn = "database"
	}
	envContent = strings.Replace(envContent, "QUEUE_CONNECTION=sync", "QUEUE_CONNECTION="+queueConn, 1)

	return os.WriteFile(filepath.Join(projectPath, ".env"), []byte(envContent), 0644)
}

// StopContainer stops a running container
func (s *DockerService) StopContainer(containerID string) error {
	cmd := exec.Command("docker", "stop", containerID)
	return cmd.Run()
}

// RemoveContainer stops and removes a container
func (s *DockerService) RemoveContainer(containerID string) error {
	exec.Command("docker", "stop", containerID).Run()
	exec.Command("docker", "rm", containerID).Run()
	return nil
}

// IsContainerHealthy checks if a container is running and healthy
func (s *DockerService) IsContainerHealthy(containerID string) bool {
	// Check container status via docker inspect
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Health.Status}}", containerID)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	
	if err := cmd.Run(); err != nil {
		// Container doesn't have health check or not running, check if it's at least running
		cmd = exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerID)
		stdout.Reset()
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return false
		}
		return strings.TrimSpace(stdout.String()) == "true"
	}
	
	status := strings.TrimSpace(stdout.String())
	// Docker health status can be: starting, healthy, unhealthy
	// We accept both "healthy" and "starting" (give it time)
	return status == "healthy" || status == "starting"
}

// RemoveImage removes a project's docker image
func (s *DockerService) RemoveImage(subdomain string) error {
	imageName := fmt.Sprintf("paas-%s", subdomain)
	// Try both with and without the paas- prefix in case naming varies
	exec.Command("docker", "rmi", imageName).Run()
	return nil
}

// PruneImages removes dangling images (labeled <none>)
func (s *DockerService) PruneImages() error {
	// Remove dangling images (<none>)
	exec.Command("docker", "image", "prune", "-f").Run()
	
	// Also remove unused project images (paas-*)
	exec.Command("docker", "image", "prune", "-a", "-f", "--filter", "label=com.paas.project=true").Run()
	
	return nil
}

// CleanupProject removes project files
func (s *DockerService) CleanupProject(subdomain string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.RemoveAll(projectPath)
}

// ===========================================
// Logs & Stats
// ===========================================

// GetContainerLogs retrieves container logs
func (s *DockerService) GetContainerLogs(containerID string, lines int) (string, error) {
	cmd := exec.Command("docker", "logs", "--tail", strconv.Itoa(lines), containerID)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to get logs: %s", stderr.String())
	}

	return stdout.String() + stderr.String(), nil
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
	// Use JSON formatting for reliable parsing
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{json .}}", containerID)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running docker stats for container %s: %v. Stderr: %s\n", containerID, err, stderr.String())
		return nil, fmt.Errorf("docker stats failed: %s", stderr.String())
	}

	// Output might contain multiple lines if multiple containers match (unlikely here)
	// or just one JSON object.
	output := strings.TrimSpace(stdout.String())
	
	if output == "" {
		fmt.Printf("Empty output from docker stats for container %s\n", containerID)
		return nil, fmt.Errorf("container not found or not running")
	}
	
	var rawStats DockerStatsJSON
	if err := json.Unmarshal([]byte(output), &rawStats); err != nil {
		fmt.Printf("Error unmarshalling docker stats: %v. Output: %s\n", err, output)
		return nil, fmt.Errorf("failed to parse docker stats: %v", err)
	}

	stats := &ContainerStats{}

	// 1. Parse CPU (remove % and trim)
	cpuStr := strings.ReplaceAll(rawStats.CPUPerc, "%", "")
	cpuVal, err := strconv.ParseFloat(strings.TrimSpace(cpuStr), 64)
	if err != nil {
		fmt.Printf("Warning: Failed to parse CPU percent '%s': %v\n", cpuStr, err)
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
		fmt.Printf("Warning: Unexpected memory format: %s\n", rawStats.MemUsage)
	}

	fmt.Printf("Stats for container %s: CPU=%.2f%%, Memory=%.2fMB/%.2fMB\n", 
		containerID, stats.CPUPercent, stats.MemoryMB, stats.MemoryMax)

	return stats, nil
}

// GetAllContainerStats retrieves resource usage for all containers
func (s *DockerService) GetAllContainerStats() (map[string]ContainerStats, error) {
	// Use custom formatting for easier parsing: ID|Name|CPU|MemUsage
	cmd := exec.Command("docker", "stats", "--no-stream", "--format", "{{.ID}}|{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running docker stats: %v. Stderr: %s\n", err, stderr.String())
		return nil, fmt.Errorf("docker stats failed: %s", stderr.String())
	}

	result := make(map[string]ContainerStats)
	output := stdout.String()
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

// GetSystemStats retrieves host machine resource usage
func (s *DockerService) GetSystemStats() (*models.SystemStats, error) {
	stats := &models.SystemStats{
		DiskPath: s.cfg.ProjectsPath,
		OS:       "Linux",
		CPUCores: 1,
	}

	// Hostname
	if hostname, err := os.Hostname(); err == nil {
		stats.Hostname = hostname
	}

	// CPU Usage
	cmd := exec.Command("sh", "-c", "top -bn1 | grep 'CPU:' | head -n1 | awk '{print $2}' | cut -d% -f1")
	if output, err := cmd.Output(); err == nil {
		cpuStr := strings.TrimSpace(string(output))
		if val, err := strconv.ParseFloat(cpuStr, 64); err == nil {
			stats.CPUUsage = val
		}
	}

	// CPU Cores
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		stats.CPUCores = strings.Count(string(data), "processor")
	}

	// Memory Usage
	cmd = exec.Command("free", "-b")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 3 {
				total, _ := strconv.ParseUint(fields[1], 10, 64)
				used, _ := strconv.ParseUint(fields[2], 10, 64)
				stats.MemoryTotal = total
				stats.MemoryUsed = used
			}
		}
	}

	// Disk Usage
	cmd = exec.Command("df", "-b", s.cfg.ProjectsPath)
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
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

	return stats, nil
}

// ListAllContainers returns all containers on the host with stats
func (s *DockerService) ListAllContainers() ([]models.DockerContainer, error) {
	// 1. Get stats for info merging
	statsMap, _ := s.GetAllContainerStats()

	// 2. Get container list with detailed info
	// Format: ID|Names|Image|State|Status|CreatedAt|IPAddress|Ports
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.ID}}|{{.Names}}|{{.Image}}|{{.State}}|{{.Status}}|{{.CreatedAt}}|{{.Networks}}|{{.Ports}}")
	output, err := cmd.Output()
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

		created, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", parts[5])

		id := parts[0]
		
		// Get IP Address via inspect if not available in format (Format doesn't easily give IP)
		ipAddr := ""
		inspectCmd := exec.Command("docker", "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", id)
		if ipOut, err := inspectCmd.Output(); err == nil {
			ipAddr = strings.TrimSpace(string(ipOut))
		}

		container := models.DockerContainer{
			ID:        id,
			Names:     strings.Split(parts[1], ","),
			Image:     parts[2],
			State:     parts[3],
			Status:    parts[4],
			CreatedAt: created,
			IPAddress: ipAddr,
			Ports:     parsePorts(parts[7]),
		}

		// Merge stats if available
		if stats, ok := statsMap[id]; ok {
			container.CPUPercent = stats.CPUPercent
			container.MemoryUsage = stats.MemoryMB
		} else {
			// Try by name since ID might be truncated in stats
			for _, name := range container.Names {
				if stats, ok := statsMap[name]; ok {
					container.CPUPercent = stats.CPUPercent
					container.MemoryUsage = stats.MemoryMB
					break
				}
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
	return strings.Split(portStr, ", ")
}

// ListAllImages returns all images on the host
func (s *DockerService) ListAllImages() ([]models.DockerImage, error) {
	// Format: ID|Repo|Tag|Size|CreatedAt
	cmd := exec.Command("docker", "images", "--format", "{{.ID}}|{{.Repository}}|{{.Tag}}|{{.Size}}|{{.CreatedAt}}")
	output, err := cmd.Output()
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
	lines := strings.Split(string(output), "\n")
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
	cmd := exec.Command("docker", "network", "ls", "--format", "{{.ID}}|{{.Name}}|{{.Driver}}|{{.Scope}}")
	output, err := cmd.Output()
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
	lines := strings.Split(string(output), "\n")
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
	cmd := exec.Command("docker", "volume", "ls", "--format", "{{.Name}}|{{.Driver}}|{{.Mountpoint}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var result []models.DockerVolume
	lines := strings.Split(string(output), "\n")
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

// Helper to convert docker memory headers (GiB, MiB, kiB, B) to MB
func parseMemoryBytes(memStr string) float64 {
	// Remove non-alphanumeric chars (except .)
	input := strings.TrimSpace(memStr)
	valueStr := ""
	unit := ""
	
	// Separate number and unit
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
		return val // Assume already MB or unknown
	}
}

// ExecLaravelCommand runs artisan commands inside container
func (s *DockerService) ExecLaravelCommand(containerID, command string) (string, error) {
	// Split command string into args to avoiding shell injection
	// This assumes the command is a space-separated list of args for artisan
	// e.g. "migrate --force" -> ["migrate", "--force"]
	args := strings.Fields(command)
	
	dockerArgs := []string{"exec", containerID, "php", "artisan"}
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.Command("docker", dockerArgs...)
	
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		output := stdout.String() + "\n" + stderr.String()
		return output, fmt.Errorf("command failed: %s", output)
	}

	return stdout.String(), nil
}

// GetEnvFile reads the .env file for a project
func (s *DockerService) GetEnvFile(subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	content, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	if err != nil {
		return "", err
	}
	return string(content), nil
}

// SaveEnvFile updates the .env file for a project
func (s *DockerService) SaveEnvFile(subdomain, content string) error {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)
	return os.WriteFile(filepath.Join(projectPath, ".env"), []byte(content), 0644)
}

// ===========================================
// Helpers
// ===========================================

// GenerateSubdomain creates a unique subdomain from project name
func GenerateSubdomain(name string) string {
	// Clean the name
	clean := strings.ToLower(name)
	clean = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(clean, "-")
	clean = strings.Trim(clean, "-")

	// Limit length
	if len(clean) > 25 {
		clean = clean[:25]
	}

	// Add random suffix
	suffix := generateRandomString(6)
	// Subdomain will be used with ProjectDomain (e.g., project.p.horn-yastudio.com)
	return fmt.Sprintf("%s-%s", clean, suffix)
}

// generateRandomString creates a random alphanumeric string
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	result := make([]byte, length)
	for i := range result {
		result[i] = charset[r.Intn(len(charset))]
	}
	return string(result)
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Create directory if needed
	os.MkdirAll(filepath.Dir(dst), 0755)

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
