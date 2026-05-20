// ===========================================
// Version Service
// ===========================================
// Detects PHP and Laravel versions from
// a project's composer.json
// ===========================================
package infrastructure

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/pkg/utils"
)

// VersionService handles framework and runtime version detection
type VersionService struct{}

// NewVersionService creates a new version service
func NewVersionService() *VersionService {
	return &VersionService{}
}

// ComposerJSON represents the relevant structure of composer.json
type ComposerJSON struct {
	Require map[string]string `json:"require"`
}

// DetectVersions reads composer.json to detect Laravel and PHP versions
func (s *VersionService) DetectVersions(projectPath string) (laravelVersion, phpVersion string, err error) {
	composerPath := filepath.Join(projectPath, "composer.json")

	// Safety Check: Ensure composer.json is a regular file and not a symlink to sensitive data.
	if !utils.IsPathWithinRoot(projectPath, composerPath) {
		return "", "", apperr.New(403, "SECURITY_VIOLATION", "Invalid project path")
	}

	if _, err := os.Stat(composerPath); os.IsNotExist(err) {
		return "", "8.4", nil // Fallback default PHP
	}

	if utils.IsSymlink(composerPath) {
		return "", "", apperr.New(403, "SECURITY_VIOLATION", "composer.json must be a regular file, not a symbolic link")
	}

	data, err := os.ReadFile(composerPath)
	if err != nil {
		return "", "", apperr.New(422, "READ_FAILED", "Failed to read composer.json: "+err.Error())
	}

	var composer ComposerJSON
	if err := json.Unmarshal(data, &composer); err != nil {
		return "", "", apperr.New(422, "INVALID_COMPOSER_JSON", "Failed to parse composer.json: "+err.Error())
	}

	laravelReq := composer.Require["laravel/framework"]
	laravelVersion = extractMajorVersion(laravelReq)

	phpReq := composer.Require["php"]
	phpVersion = detectPHPVersion(laravelVersion, phpReq)

	return laravelVersion, phpVersion, nil
}

// PackageJSON represents the relevant structure of package.json
type PackageJSON struct {
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
}

// DetectNodeVersion reads package.json to detect the Node.js version
func (s *VersionService) DetectNodeVersion(projectPath string) (string, error) {
	packagePath := filepath.Join(projectPath, "package.json")

	if !utils.IsPathWithinRoot(projectPath, packagePath) {
		return "", apperr.New(403, "SECURITY_VIOLATION", "Invalid project path")
	}

	if _, err := os.Stat(packagePath); os.IsNotExist(err) {
		return "", nil
	}

	if utils.IsSymlink(packagePath) {
		return "", apperr.New(403, "SECURITY_VIOLATION", "package.json must be a regular file, not a symbolic link")
	}

	data, err := os.ReadFile(packagePath)
	if err != nil {
		return "", apperr.New(422, "READ_FAILED", "Failed to read package.json: "+err.Error())
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return "", apperr.New(422, "INVALID_PACKAGE_JSON", "Failed to parse package.json: "+err.Error())
	}

	if pkg.Engines.Node != "" {
		re := regexp.MustCompile(`(\d+(\.\d+)?)`)
		matches := re.FindStringSubmatch(pkg.Engines.Node)
		if len(matches) > 0 {
			return matches[0], nil
		}
	}

	return "20", nil // Default to Node.js 20
}

// DetectGoVersion reads go.mod to detect the Go version
func (s *VersionService) DetectGoVersion(projectPath string) (string, error) {
	goModPath := filepath.Join(projectPath, "go.mod")

	if !utils.IsPathWithinRoot(projectPath, goModPath) {
		return "", nil
	}

	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return "", nil
	}

	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", nil
	}

	re := regexp.MustCompile(`go\s+(\d+\.\d+)`)
	matches := re.FindStringSubmatch(string(data))
	if len(matches) > 1 {
		return matches[1], nil
	}

	return "1.22", nil // Default to Go 1.22
}

// DetectPythonVersion attempts to find Python version
func (s *VersionService) DetectPythonVersion(projectPath string) (string, error) {
	// Common files to check: runtime.txt (Heroku/standard) or .python-version
	runtimePath := filepath.Join(projectPath, "runtime.txt")
	if data, err := os.ReadFile(runtimePath); err == nil {
		re := regexp.MustCompile(`python-(\d+\.\d+)`)
		matches := re.FindStringSubmatch(string(data))
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "3.11", nil // Default to Python 3.11
}

// DetectRuntimeVersion is a generic method to detect version based on framework
func (s *VersionService) DetectRuntimeVersion(projectPath string, framework string) (string, error) {
	switch framework {
	case "Laravel", "PHP":
		_, php, _ := s.DetectVersions(projectPath)
		return php, nil
	case "Node.js", "Next.js", "Vite", "React", "Vue", "Nuxt.js", "Svelte", "Angular", "TypeScript":
		return s.DetectNodeVersion(projectPath)
	case "Go":
		return s.DetectGoVersion(projectPath)
	case "Python":
		return s.DetectPythonVersion(projectPath)
	default:
		// Attempt to guess if framework is unknown
		if _, err := os.Stat(filepath.Join(projectPath, "package.json")); err == nil {
			return s.DetectNodeVersion(projectPath)
		}
		if _, err := os.Stat(filepath.Join(projectPath, "composer.json")); err == nil {
			_, php, _ := s.DetectVersions(projectPath)
			return php, nil
		}
		if _, err := os.Stat(filepath.Join(projectPath, "go.mod")); err == nil {
			return s.DetectGoVersion(projectPath)
		}
		return "", nil
	}
}

// extractMajorVersion extracts the major version number from a composer constraint
func extractMajorVersion(constraint string) string {
	re := regexp.MustCompile(`(\d+)`)
	matches := re.FindStringSubmatch(constraint)
	if len(matches) > 0 {
		return matches[0]
	}
	return "11" // Default to Laravel 11
}

// detectPHPVersion determines the appropriate PHP version based on Laravel version
// or falls back to parsing the PHP constraint from composer.json
func detectPHPVersion(laravelVersion, phpConstraint string) string {
	// Laravel version to minimum PHP version mapping
	laravelPHPMap := map[string]string{
		"8":  "8.0",
		"9":  "8.1",
		"10": "8.2",
		"11": "8.3",
		"12": "8.4",
		"13": "8.4",
		"14": "8.5",
	}

	if php, ok := laravelPHPMap[laravelVersion]; ok {
		return php
	}

	// Fallback: extract from PHP constraint string
	re := regexp.MustCompile(`(\d+\.\d+)`)
	matches := re.FindStringSubmatch(phpConstraint)
	if len(matches) > 1 {
		return matches[1]
	}

	return "8.4" // Default to PHP 8.4

}

// FormatVersionLabel returns a display-friendly version label
func FormatVersionLabel(laravelVersion, phpVersion string) string {
	return fmt.Sprintf("Laravel %s / PHP %s", laravelVersion, phpVersion)
}
