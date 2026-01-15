// ===========================================
// Docker Service
// ===========================================
// Manages Docker containers for student projects
// ===========================================
package services

import (
	"bytes"
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
func (s *DockerService) CloneRepository(githubURL, subdomain string) (string, error) {
	projectPath := filepath.Join(s.cfg.ProjectsPath, subdomain)

	// Remove existing directory if present
	os.RemoveAll(projectPath)

	// Clone the repository
	cmd := exec.Command("git", "clone", "--depth=1", githubURL, projectPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone failed: %s", stderr.String())
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
func (s *DockerService) BuildAndRun(project *models.Project, phpVersion string) (string, error) {
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

	// Create .env file for Laravel
	if err := s.createEnvFile(project, projectPath); err != nil {
		return "", fmt.Errorf("failed to create .env: %w", err)
	}

	// Build image
	imageName := fmt.Sprintf("paas-%s", project.Subdomain)
	cmd := exec.Command("docker", "build", "-t", imageName, projectPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker build failed: %s", stderr.String())
	}

	// Generate container name
	containerName := fmt.Sprintf("paas-project-%s", project.Subdomain)

	// Remove existing container if present
	exec.Command("docker", "rm", "-f", containerName).Run()

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
			project.Subdomain, project.Subdomain, s.cfg.BaseDomain),
		"--label", fmt.Sprintf("traefik.http.routers.%s.entrypoints=websecure", project.Subdomain),
		"--label", fmt.Sprintf("traefik.http.routers.%s.tls.certresolver=letsencrypt", project.Subdomain),
		"--label", "traefik.http.services." + project.Subdomain + ".loadbalancer.server.port=80",
		imageName,
	}

	cmd = exec.Command("docker", runArgs...)
	var stdout bytes.Buffer
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
func (s *DockerService) createEnvFile(project *models.Project, projectPath string) error {
	envContent := fmt.Sprintf(`APP_NAME="%s"
APP_ENV=production
APP_DEBUG=false
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
		project.Subdomain, s.cfg.BaseDomain,
		project.DatabaseName,
		project.DatabaseName,
		project.DatabaseName,
	)

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

// GetContainerStats retrieves container resource usage
func (s *DockerService) GetContainerStats(containerID string) (*ContainerStats, error) {
	cmd := exec.Command("docker", "stats", "--no-stream", "--format",
		`{"cpu":"{{.CPUPerc}}","mem":"{{.MemUsage}}"}`, containerID)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// Parse the output
	output := stdout.String()
	stats := &ContainerStats{}

	// Extract CPU percentage
	cpuRe := regexp.MustCompile(`"cpu":"([\d.]+)%"`)
	if matches := cpuRe.FindStringSubmatch(output); len(matches) > 1 {
		stats.CPUPercent, _ = strconv.ParseFloat(matches[1], 64)
	}

	// Extract memory usage
	memRe := regexp.MustCompile(`"mem":"([\d.]+)MiB / ([\d.]+)MiB"`)
	if matches := memRe.FindStringSubmatch(output); len(matches) > 2 {
		stats.MemoryMB, _ = strconv.ParseFloat(matches[1], 64)
		stats.MemoryMax, _ = strconv.ParseFloat(matches[2], 64)
	}

	return stats, nil
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
	if len(clean) > 20 {
		clean = clean[:20]
	}

	// Add random suffix
	suffix := generateRandomString(6)
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
