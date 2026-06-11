// ===========================================
// Filesystem Utilities
// ===========================================
// Security-focused helpers for path validation
// and safe file operations.
// ===========================================
package utils

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

// WriteFileAtomic writes data to a file atomically by writing to a temporary file,
// performing fsync, and then doing an atomic rename to the target path.
func WriteFileAtomic(filename string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(filename)
	// Create a temp file in the same directory to guarantee atomic rename (same mount/device)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() {
		// Clean up the temp file if any error occurs
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmpFile.Chmod(perm); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if _, err = tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return err
	}

	// Perform fsync to ensure persistence
	if err = tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return err
	}

	if err = tmpFile.Close(); err != nil {
		return err
	}

	// Atomic rename replaces the target file atomically on Unix systems
	if err = os.Rename(tmpName, filename); err != nil {
		return err
	}

	return nil
}

// PruneJobLogs finds all files matching the pattern in logsDir and removes the oldest ones,
// keeping only the maxKeep most recent files (based on modification time).
func PruneJobLogs(logsDir string, pattern string, maxKeep int) error {
	if maxKeep <= 0 {
		return nil
	}

	matches, err := filepath.Glob(filepath.Join(logsDir, pattern))
	if err != nil {
		return err
	}

	if len(matches) <= maxKeep {
		return nil
	}

	type fileInfo struct {
		path    string
		modTime time.Time
	}

	files := make([]fileInfo, 0, len(matches))
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    path,
			modTime: info.ModTime(),
		})
	}

	// Sort descending (newest first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	// Remove older files
	for i := maxKeep; i < len(files); i++ {
		_ = os.Remove(files[i].path)
	}

	return nil
}

// TruncateFileIfNeeded checks if the file at path exceeds maxSizeBytes.
// If it does, it keeps the last 50% of the maxSizeBytes bytes of the file and truncates the rest.
func TruncateFileIfNeeded(path string, maxSizeBytes int64) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Size() <= maxSizeBytes {
		return nil
	}

	// Keep the last 50% of the maxSizeBytes
	keepBytes := maxSizeBytes / 2
	if keepBytes <= 0 {
		return os.Truncate(path, 0)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read last keepBytes
	offset := info.Size() - keepBytes
	if offset < 0 {
		offset = 0
	}

	buf := make([]byte, keepBytes)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return err
	}

	// Write back atomically
	return WriteFileAtomic(path, buf[:n], info.Mode())
}
