package docker

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/laravel-paas/shared/apperr"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/infrastructure"
	"github.com/laravel-paas/shared/pkg/utils"
	"gorm.io/gorm"
)

type DockerService struct {
	cfg     *config.Config
	storage *infrastructure.StorageService
	db      *gorm.DB
}

func (s *DockerService) ResolveBuildPath(projectPath, baseDirectory string) (string, error) {
	if strings.TrimSpace(baseDirectory) == "" {
		return s.GetBuildPath(projectPath), nil
	}

	clean := filepath.Clean(baseDirectory)
	if filepath.IsAbs(clean) || clean == "." || clean == string(os.PathSeparator) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) || strings.ContainsRune(baseDirectory, '\\') {
		return "", apperr.New(400, "INVALID_BASE_DIRECTORY", "Configured base directory is invalid.")
	}

	candidate := filepath.Join(projectPath, clean)
	if !utils.IsPathWithinRoot(projectPath, candidate) {
		return "", apperr.New(400, "INVALID_BASE_DIRECTORY", "Configured base directory is invalid.")
	}
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate, nil
	}
	return "", apperr.New(400, "INVALID_BASE_DIRECTORY", "Configured base directory does not exist.")
}

func NewDockerService(cfg *config.Config, storage *infrastructure.StorageService, db *gorm.DB) *DockerService {
	return &DockerService{cfg: cfg, storage: storage, db: db}
}

func (s *DockerService) GetDB() *gorm.DB {
	return s.db
}

func (s *DockerService) GetBuildPath(root string) string {
	markers := map[string]int{
		"artisan":          100,
		"composer.json":    80,
		"index.php":        80,
		"go.mod":           80,
		"go.work":          80,
		"pyproject.toml":   80,
		"requirements.txt": 80,
		"Pipfile":          80,
		"Gemfile":          80,
		"Cargo.toml":       80,
		"pom.xml":          80,
		"build.gradle":     80,
		"build.gradle.kts": 80,
		"mix.exs":          80,
		"deno.json":        80,
		"deno.jsonc":       80,
		"gleam.toml":       80,
		"CMakeLists.txt":   80,
		"meson.build":      80,
		"package.json":     50,
		"start.sh":         50,
		"index.html":       30,
		"Staticfile":       30,
	}
	markerPatterns := map[string]int{"*.csproj": 80}
	priority := map[string]int{"backend": 15, "app": 14, "server": 13, "api": 12, "frontend": 11, "web": 10, "ui": 9}
	type candidate struct {
		path  string
		bonus int
	}
	candidates := []candidate{{path: root, bonus: 20}}
	if entries, err := os.ReadDir(root); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" || entry.Name() == "vendor" {
				continue
			}
			candidates = append(candidates, candidate{path: filepath.Join(root, entry.Name()), bonus: priority[entry.Name()]})
		}
	}

	bestPath := root
	bestScore := 0
	for _, candidate := range candidates {
		strongestMarker := 0
		secondMarker := 0
		for marker, weight := range markers {
			if info, err := os.Lstat(filepath.Join(candidate.path, marker)); err == nil && info.Mode().IsRegular() {
				strongestMarker, secondMarker = rankMarker(weight, strongestMarker, secondMarker)
			}
		}
		for pattern, weight := range markerPatterns {
			if matches, _ := filepath.Glob(filepath.Join(candidate.path, pattern)); len(matches) > 0 {
				strongestMarker, secondMarker = rankMarker(weight, strongestMarker, secondMarker)
			}
		}
		markerScore := strongestMarker + secondMarker/4
		score := candidate.bonus + markerScore
		if markerScore > 0 && score > bestScore {
			bestPath = candidate.path
			bestScore = score
		}
	}
	return bestPath
}

func rankMarker(weight, strongest, second int) (int, int) {
	if weight > strongest {
		return weight, strongest
	}
	if weight > second {
		return strongest, weight
	}
	return strongest, second
}
