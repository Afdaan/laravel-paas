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
command=php /var/www/html/artisan queue:work database --sleep=3 --tries=3
autostart=true
autorestart=true
user=www-data
numprocs=1
redirect_stderr=true
stdout_logfile=/var/www/html/storage/logs/worker.log
`
		f, err := os.OpenFile(filepath.Join(projectPath, "docker", "supervisord.conf"), os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			if _, err := f.WriteString(workerConfig); err != nil {
				f.Close()
				return "", fmt.Errorf("failed to append supervisor config: %w", err)
			}
			f.Close()
		}
	}

	// Create .env file for Laravel
	if err := s.createEnvFile(project, projectPath, projectDomain); err != nil {
		return "", fmt.Errorf("failed to create .env: %w", err)
	}

	// Build image
	imageName := fmt.Sprintf("paas-%s", project.Subdomain)

	var stdout, stderr bytes.Buffer

	buildArgs := []string{"buildx", "build", "--load", "-t", imageName, projectPath}
	cmd := exec.Command("docker", buildArgs...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Fallback to classic build for environments without buildx
		stdout.Reset()
		stderr.Reset()

		cmd = exec.Command("docker", "build", "-t", imageName, projectPath)
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err2 := cmd.Run(); err2 != nil {
			return "", fmt.Errorf("docker build failed: %s%s", stdout.String(), stderr.String())
		}
	}

	// Generate unique container name for zero-downtime deployment
	timestamp := time.Now().Unix()
	containerName := fmt.Sprintf("paas-project-%s-%d", project.Subdomain, timestamp)

	// Run container with Traefik labels for automatic SSL
	runArgs := []string{
		"run", "-d",
		"--name", containerName,
		"--network", s.cfg.DockerNetwork,
		"--restart", "unless-stopped",
		// Resource limits
		"--cpus", "0.5",
		"--memory", "512m",
		// Traefik labels for automatic SSL
		"--label", "traefik.enable=true",
		"--label", fmt.Sprintf("traefik.http.routers.%s.rule=Host(`%s.%s`)",
			project.Subdomain, project.Subdomain, projectDomain),
		"--label", "traefik.http.services." + project.Subdomain + ".loadbalancer.server.port=80",
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

// RemoveImage removes a project's docker image
func (s *DockerService) RemoveImage(subdomain string) error {
	imageName := fmt.Sprintf("paas-%s", subdomain)
	// Try both with and without the paas- prefix in case naming varies
	exec.Command("docker", "rmi", imageName).Run()
	return nil
}

// PruneImages removes dangling images (labeled <none>)
func (s *DockerService) PruneImages() error {
	// docker image prune -f removes dangling images
	return exec.Command("docker", "image prune", "-f").Run()
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
