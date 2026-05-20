package utils

import (
	"fmt"
	"strings"
)

// frameworkCommands defines the security rules for a specific framework/runtime
type frameworkCommands struct {
	Allowed         map[string]bool
	BlockedPatterns []string
}

// commandRegistry stores security rules for all supported frameworks
var commandRegistry = map[string]frameworkCommands{
	"Laravel": {
		Allowed: map[string]bool{
			"cache:clear":      true,
			"cache:forget":     true,
			"config:cache":     true,
			"config:clear":     true,
			"route:cache":      true,
			"route:clear":      true,
			"route:list":       true,
			"view:cache":       true,
			"view:clear":       true,
			"optimize":         true,
			"optimize:clear":   true,
			"list":             true,
			"about":            true,
			"env":              true,
			"storage:link":     true,
			"key:generate":     true,
			"migrate":          true,
			"db:seed":          true,
			"livewire:publish": true,
		},
		BlockedPatterns: []string{
			"migrate:fresh",
			"migrate:reset",
			"migrate:rollback",
			"tinker",
			"make:",
			"down",
			"up",
			"serve",
			"schedule:run",
			"schedule:work",
			"queue:work",
			"queue:listen",
			"queue:restart",
			"queue:retry",
			"queue:forget",
			"queue:flush",
			"queue:prune-batches",
			"optimize:v2",
			"stub:publish",
			"vendor:publish",
			"install",
			"test",
			"pest",
			"clear-compiled",
		},
	},
	"Node.js": {
		Allowed: map[string]bool{
			"install": true,
			"build":   true,
			"start":   true,
			"test":    true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"npm publish",
			"yarn publish",
		},
	},
}

// ValidateCommand checks if a command is allowed for a given framework
func ValidateCommand(framework string, command string) error {
	rules, exists := commandRegistry[framework]
	if !exists {
		// Default behavior for unknown frameworks: reject all for safety
		return fmt.Errorf("security rules not defined for framework: %s", framework)
	}

	// Normalize command: trim whitespace and get the base command (first word)
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// For security, we usually only care about the first part of the command
	// e.g. "migrate --force" -> "migrate"
	parts := strings.Fields(trimmed)
	baseCommand := parts[0]

	// 1. Check against blocked patterns (Prefix match)
	for _, pattern := range rules.BlockedPatterns {
		if baseCommand == pattern || strings.HasPrefix(baseCommand, pattern) {
			return fmt.Errorf("command '%s' is restricted for security reasons", baseCommand)
		}
	}

	// 2. Check against allowlist (Exact match)
	if !rules.Allowed[baseCommand] {
		return fmt.Errorf("command '%s' is not in the allowed list for %s", baseCommand, framework)
	}

	return nil
}
