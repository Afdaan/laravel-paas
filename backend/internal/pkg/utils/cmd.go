// ===========================================
// Command Utilities
// ===========================================
// Provides timeout-safe wrappers for os/exec
// to prevent goroutine leaks from hung processes
// ===========================================
package utils

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// DefaultTimeout is the standard deadline for shell commands
const DefaultTimeout = 2 * time.Minute

// Result holds the output of an executed command
type Result struct {
	Stdout string
	Stderr string
}

// Run executes a command with a timeout.
// If the command does not finish within the deadline, the process is killed.
func Run(timeout time.Duration, name string, args ...string) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := &Result{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %s: %s %v", timeout, name, args)
	}

	return result, err
}

// RunSilent executes a command with a timeout but discards output.
// Used for fire-and-forget operations like cleanup.
func RunSilent(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	err := cmd.Run()

	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out after %s: %s %v", timeout, name, args)
	}

	return err
}
