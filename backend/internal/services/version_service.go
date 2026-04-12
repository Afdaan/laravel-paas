// ===========================================
// Version Service
// ===========================================
// Detects PHP and Laravel versions from
// a project's composer.json
// ===========================================
package services

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/laravel-paas/backend/internal/apperr"
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

	data, err := os.ReadFile(composerPath)
	if err != nil {
		return "", "", apperr.New(422, "MISSING_COMPOSER_JSON", "Project is missing composer.json: "+err.Error())
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
