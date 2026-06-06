package docker

import (
	"os"
	"strings"
)

// ParseProjectEnv reads a .env file and returns a map of key-values.
// It handles basic environment variable parsing including comments and quotes.
func (s *DockerService) ParseProjectEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	envVars := make(map[string]string)
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Split by the first '=' character
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove surrounding quotes if they exist (e.g., "value" or 'value')
		value = strings.Trim(value, "\"'")

		envVars[key] = value
	}

	return envVars, nil
}
