// ===========================================
// Filesystem Utilities
// ===========================================
// Security-focused helpers for path validation
// and safe file operations.
// ===========================================
package utils

import (
	"os"
	"path/filepath"
	"strings"
)

// IsPathWithinRoot verifies that a candidate path is physically located
// within a root directory, resolving all symlinks to prevent path traversal attacks.
func IsPathWithinRoot(root, candidate string) bool {
	// Resolve symlinks for both root and candidate to find their absolute, physical locations.
	// This prevents escaping via symlinks (e.g. candidate -> /etc/shadow).
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootResolved, _ = filepath.Abs(root)
	}

	canResolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		canResolved, _ = filepath.Abs(candidate)
	}

	rootResolved = filepath.Clean(rootResolved)
	canResolved = filepath.Clean(canResolved)

	// A path is within root if it IS the root...
	if canResolved == rootResolved {
		return true
	}

	// ...or if it starts with the root directory as a prefix.
	// We add a separator to the prefix to ensure we don't match partial folder names
	// (e.g. /app-data vs /app).
	prefix := rootResolved + string(os.PathSeparator)
	return strings.HasPrefix(canResolved, prefix)
}

// IsSymlink checks if a given path is a symbolic link.
func IsSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}
