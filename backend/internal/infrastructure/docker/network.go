package docker

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/laravel-paas/backend/internal/pkg/utils"
)

// DetectExposedPort inspects a docker image to find the first EXPOSEd port.
// Returns the port number and nil if found, or 0 and error if not.
func (s *DockerService) DetectExposedPort(imageName string) (int, error) {
	// docker image inspect <image> --format '{{json .Config.ExposedPorts}}'
	res, err := utils.Run(10*time.Second, "docker", "image", "inspect", imageName, "--format", "{{json .Config.ExposedPorts}}")
	if err != nil {
		return 0, err
	}

	output := strings.TrimSpace(res.Stdout)
	if output == "null" || output == "" || output == "{}" {
		return 0, fmt.Errorf("no exposed ports found in image metadata")
	}

	// Output format: {"3000/tcp":{},"80/tcp":{}}
	var exposed map[string]interface{}
	if err := json.Unmarshal([]byte(output), &exposed); err != nil {
		return 0, err
	}

	// 1. Collect and sort all available ports
	var ports []int
	for portKey := range exposed {
		parts := strings.Split(portKey, "/")
		if len(parts) > 0 {
			var p int
			if _, err := fmt.Sscanf(parts[0], "%d", &p); err == nil && p > 0 {
				ports = append(ports, p)
			}
		}
	}

	if len(ports) == 0 {
		return 0, fmt.Errorf("no valid ports parsed from metadata")
	}

	// 2. Prioritize common web ports
	priorityPorts := []int{80, 8080, 3000, 5000}
	for _, p := range priorityPorts {
		for _, available := range ports {
			if available == p {
				return available, nil
			}
		}
	}

	// 3. If no priority port found, pick the smallest one but AVOID 9000 if alternatives exist
	// (9000 is usually PHP-FPM FastCGI, not HTTP)
	var fallback int
	for _, p := range ports {
		if p == 9000 && len(ports) > 1 {
			continue
		}
		if fallback == 0 || p < fallback {
			fallback = p
		}
	}

	if fallback > 0 {
		return fallback, nil
	}

	return ports[0], nil
}
