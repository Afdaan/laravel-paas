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
			"migrate:fresh":    true,
			"db:seed":          true,
			"livewire:publish": true,
		},
		BlockedPatterns: []string{
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
			"npm":  true,
			"npx":  true,
			"pnpm": true,
			"yarn": true,
			"bun":  true,
			"node": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"npm publish",
			"pnpm publish",
			"yarn publish",
			"bun publish",
		},
	},
	"Go": {
		Allowed: map[string]bool{
			"go":   true,
			"make": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"go install",
		},
	},
	"Python": {
		Allowed: map[string]bool{
			"python":   true,
			"python3":  true,
			"pip":      true,
			"pip3":     true,
			"poetry":   true,
			"flask":    true,
			"django":   true,
			"manage":   true,
			"celery":   true,
			"gunicorn": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"pip install",
		},
	},
	"Ruby": {
		Allowed: map[string]bool{
			"ruby":    true,
			"rails":   true,
			"rake":    true,
			"bundle":  true,
			"bundler": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"gem push",
		},
	},
	"Rust": {
		Allowed: map[string]bool{
			"cargo": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"cargo publish",
		},
	},
	"Java": {
		Allowed: map[string]bool{
			"java":      true,
			"mvn":       true,
			"gradle":    true,
			"./gradlew": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"mvn deploy",
			"gradle publish",
		},
	},
	"PHP": {
		Allowed: map[string]bool{
			"php":      true,
			"composer": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"composer global",
		},
	},
	"Static": {
		Allowed: map[string]bool{
			"npm":  true,
			"npx":  true,
			"pnpm": true,
			"yarn": true,
			"bun":  true,
			"node": true,
		},
		BlockedPatterns: []string{
			"rm -rf",
			"npm publish",
		},
	},
}

var javascriptFrameworks = map[string]string{
	"Next.js":    "Node.js",
	"Nuxt.js":    "Node.js",
	"Vite":       "Node.js",
	"React":      "Node.js",
	"Vue":        "Node.js",
	"Svelte":     "Node.js",
	"Angular":    "Node.js",
	"TypeScript": "Node.js",
	"Golang":     "Go",
}

// ValidateCommand checks if a command is allowed for a given framework
func ValidateCommand(framework string, command string) error {
	if normalized, ok := javascriptFrameworks[framework]; ok {
		framework = normalized
	}

	rules, exists := commandRegistry[framework]
	if !exists {
		framework = "Node.js"
		rules = commandRegistry[framework]
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
		if trimmed == pattern || strings.HasPrefix(trimmed, pattern+" ") || strings.Contains(trimmed, "&& "+pattern) || strings.Contains(trimmed, "; "+pattern) {
			return fmt.Errorf("command '%s' is restricted for security reasons", pattern)
		}
	}

	// 2. Check against allowlist (Exact match)
	if !rules.Allowed[baseCommand] {
		return fmt.Errorf("command '%s' is not in the allowed list for %s", baseCommand, framework)
	}

	return nil
}
