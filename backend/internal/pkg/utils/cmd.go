// ===========================================
// Command Utilities
// ===========================================
// Provides timeout-safe wrappers for os/exec
// to prevent goroutine leaks from hung processes.
// ===========================================
package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
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
	return RunInDirWithEnv(timeout, "", nil, name, args...)
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

// RunWithLog executes a command with a timeout and writes stdout and stderr to a specified log file.
func RunWithLog(timeout time.Duration, logFilePath string, name string, args ...string) (*Result, error) {
	return RunInDirWithEnvWithLog(timeout, "", nil, logFilePath, name, args...)
}

// RunWithRefinedLog executes a command with a timeout and writes filtered stdout and stderr to a specified log file.
func RunWithRefinedLog(timeout time.Duration, logFilePath string, name string, args ...string) (*Result, error) {
	return RunInDirWithEnvWithRefinedLog(timeout, "", nil, logFilePath, name, args...)
}

// RunInDirWithEnv executes a command in a specific directory with optional environment variables.
func RunInDirWithEnv(timeout time.Duration, dir string, env []string, name string, args ...string) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}

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

// RunInDirWithEnvWithLog executes a command in a specific directory with optional env vars and logs to a file.
func RunInDirWithEnvWithLog(timeout time.Duration, dir string, env []string, logFilePath string, name string, args ...string) (*Result, error) {
	return runInDirWithEnvWithLog(timeout, dir, env, logFilePath, false, name, args...)
}

// RunInDirWithEnvWithRefinedLog executes a command in a specific directory with optional env vars and logs filtered output to a file.
func RunInDirWithEnvWithRefinedLog(timeout time.Duration, dir string, env []string, logFilePath string, name string, args ...string) (*Result, error) {
	return runInDirWithEnvWithLog(timeout, dir, env, logFilePath, true, name, args...)
}

func runInDirWithEnvWithLog(timeout time.Duration, dir string, env []string, logFilePath string, refined bool, name string, args ...string) (*Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	// Open log file, create or overwrite
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logFilePath, err)
	}
	defer logFile.Close()

	var output io.Writer = logFile
	var refiner *LogRefiner
	if refined {
		refiner = NewLogRefiner(logFile)
		output = refiner
	}

	cmd.Stdout = io.MultiWriter(&stdoutBuf, output)
	cmd.Stderr = io.MultiWriter(&stderrBuf, output)

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	if refiner != nil {
		_ = refiner.Flush()
	}

	summaryMsg := fmt.Sprintf("\n========================================================================\n[BUILD SUMMARY] Application built successfully in %s\n========================================================================\n", formatDuration(duration))
	_, _ = logFile.WriteString(summaryMsg)

	result := &Result{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %s: %s %v", timeout, name, args)
	}

	return result, err
}

// RunCtx executes a command with a parent context and timeout.
func RunCtx(parentCtx context.Context, timeout time.Duration, name string, args ...string) (*Result, error) {
	return RunInDirWithEnvCtx(parentCtx, timeout, "", nil, name, args...)
}

// RunInDirWithEnvCtx executes a command with a parent context, directory, and env vars.
func RunInDirWithEnvCtx(parentCtx context.Context, timeout time.Duration, dir string, env []string, name string, args ...string) (*Result, error) {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}

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
	if ctx.Err() == context.Canceled {
		return result, fmt.Errorf("command cancelled: %s %v", name, args)
	}

	return result, err
}

// RunWithRefinedLogCtx executes a command with a parent context, timeout, and writes filtered output to a log file.
func RunWithRefinedLogCtx(parentCtx context.Context, timeout time.Duration, logFilePath string, name string, args ...string) (*Result, error) {
	return runInDirWithEnvWithLogCtx(parentCtx, timeout, "", nil, logFilePath, true, name, args...)
}

func runInDirWithEnvWithLogCtx(parentCtx context.Context, timeout time.Duration, dir string, env []string, logFilePath string, refined bool, name string, args ...string) (*Result, error) {
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logFilePath, err)
	}
	defer logFile.Close()

	var output io.Writer = logFile
	var refiner *LogRefiner
	if refined {
		refiner = NewLogRefiner(logFile)
		output = refiner
	}

	cmd.Stdout = io.MultiWriter(&stdoutBuf, output)
	cmd.Stderr = io.MultiWriter(&stderrBuf, output)

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	if refiner != nil {
		_ = refiner.Flush()
	}

	summaryMsg := fmt.Sprintf("\n========================================================================\n[BUILD SUMMARY] Application built successfully in %s\n========================================================================\n", formatDuration(duration))
	_, _ = logFile.WriteString(summaryMsg)

	result := &Result{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	if ctx.Err() == context.DeadlineExceeded {
		return result, fmt.Errorf("command timed out after %s: %s %v", timeout, name, args)
	}
	if ctx.Err() == context.Canceled {
		return result, fmt.Errorf("command cancelled: %s %v", name, args)
	}

	return result, err
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

