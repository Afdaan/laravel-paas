package utils

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestDeploymentLogCapConcurrentWriters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "build-job.log")
	first, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := first.Truncate(DeploymentLogMaxBytes - 1024); err != nil {
		t.Fatal(err)
	}
	second, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var writers sync.WaitGroup
	for index := 0; index < 16; index++ {
		file := []*os.File{first, second}[index%2]
		writers.Add(1)
		go func() {
			defer writers.Done()
			if _, err := WriteDeploymentLog(file, strings.Repeat("output", 100)); err != nil {
				t.Error(err)
			}
		}()
	}
	writers.Wait()
	assertCappedDeploymentLog(t, path)
	if truncated, err := WriteDeploymentLog(first, "after limit\n"); err != nil || !truncated {
		t.Fatalf("append after cap: truncated=%v err=%v", truncated, err)
	}
	assertCappedDeploymentLog(t, path)
}

func assertCappedDeploymentLog(t *testing.T, path string) {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.Size() != DeploymentLogMaxBytes {
		t.Fatalf("log cap: info=%v err=%v", info, err)
	}
	if _, err := file.Seek(-1024, io.SeekEnd); err != nil {
		t.Fatal(err)
	}
	tail, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(tail), DeploymentLogTruncatedMessage) != 1 || !bytes.HasSuffix(tail, []byte(DeploymentLogTruncatedMessage+"\n")) {
		t.Fatalf("missing/duplicate truncation marker: %q", tail)
	}
}

func TestDeploymentLogCapProcessesAndExistingOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "infra-job.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(DeploymentLogMaxBytes + 1024); err != nil {
		t.Fatal(err)
	}
	file.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var processes []*exec.Cmd
	for index := 0; index < 2; index++ {
		process := exec.Command(executable, "-test.run=^TestBuildOutputProcess$")
		process.Env = append(os.Environ(), "BUILD_OUTPUT_HELPER=append", "BUILD_LOG_PATH="+path)
		if err := process.Start(); err != nil {
			t.Fatal(err)
		}
		processes = append(processes, process)
	}
	for _, process := range processes {
		if err := process.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	assertCappedDeploymentLog(t, path)
}

func TestDeploymentLogReportsWriteFailure(t *testing.T) {
	file, err := os.OpenFile("/dev/full", os.O_WRONLY, 0)
	if err != nil {
		t.Skip("/dev/full unavailable")
	}
	defer file.Close()
	if _, err := WriteDeploymentLog(file, "output\n"); !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("disk-full error lost: %v", err)
	}
}

func TestBuildOutputTailBoundsAndSecretBoundary(t *testing.T) {
	var tail buildOutputTail
	limit := buildOutputMaxBytes - len(buildOutputTruncatedMessage)
	output := "credential=TOPSECRET\n" + strings.Repeat("x", limit-len("SECRET\n"))
	if written, err := tail.Write([]byte(output)); written != len(output) || err != nil {
		t.Fatalf("capture: written=%d err=%v", written, err)
	}
	if cap(tail.data) > limit || len(tail.String()) > buildOutputMaxBytes || strings.Contains(tail.String(), "SECRET") {
		t.Fatal("capture exceeded ceiling or exposed a partial secret")
	}
	tail.Write([]byte("\nfinal diagnostic\n"))
	if !strings.HasSuffix(tail.String(), "final diagnostic\n") {
		t.Fatal("diagnostic tail lost")
	}
}

func TestBuildLineWriterDropsWholeOversizedLine(t *testing.T) {
	var output bytes.Buffer
	writer := &buildLineWriter{writer: &output}
	for _, part := range []string{"short", " line\n", strings.Repeat("x", buildLineMaxBytes-3) + "TOP", "SECRET", "\nnext line"} {
		if written, err := writer.Write([]byte(part)); written != len(part) || err != nil {
			t.Fatalf("line write: written=%d err=%v", written, err)
		}
		if len(writer.data) > buildLineMaxBytes {
			t.Fatal("line buffer exceeded ceiling")
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if want := "short line\n" + buildLineTruncatedMessage + "next line\n"; output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestBuildRunnerDrainsOutputAndKeepsDiagnostics(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"success", "failure", "file", "refined"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "build-job.log")
			file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			var missingStartCommand atomic.Bool
			var finalSummary atomic.Bool
			observeLog := func(message string) {
				if strings.Contains(message, "No start command detected") {
					missingStartCommand.Store(true)
				}
			}
			var callback func(string)
			if mode != "file" {
				callback = func(message string) {
					if strings.HasPrefix(message, "✓ Build completed in ") || strings.HasPrefix(message, "✗ Build failed in ") {
						finalSummary.Store(true)
					}
					message = strings.ReplaceAll(message, "TOPSECRET", "[REDACTED]")
					if _, err := WriteDeploymentLog(file, message+"\n"); err != nil {
						t.Error(err)
					}
				}
			}
			env := []string{"BUILD_OUTPUT_HELPER=" + mode}
			result, err := runInDirWithEnvWithLogCtx(context.Background(), 15*time.Second, "", env, path, mode == "refined", callback, observeLog, executable, "-test.run=^TestBuildOutputProcess$")
			if (err != nil) != (mode == "failure") {
				t.Fatalf("unexpected command error: %v", err)
			}
			if len(result.Stdout) > buildOutputMaxBytes || len(result.Stderr) > buildOutputMaxBytes {
				t.Fatal("command capture exceeded ceiling")
			}
			if !strings.HasSuffix(result.Stdout, "final stdout diagnostic\n") || !strings.HasSuffix(result.Stderr, "final stderr diagnostic\n") {
				t.Fatal("final diagnostic lost")
			}
			if mode != "file" && (!missingStartCommand.Load() || !finalSummary.Load()) {
				t.Fatal("streamed build signal or final summary lost")
			}
			if mode == "refined" {
				output, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(output), "TOPSECRET") || strings.Count(string(output), strings.TrimSpace(buildLineTruncatedMessage)) != 2 {
					t.Fatal("refined output leaked a secret or lost oversized-line markers")
				}
			} else {
				assertCappedDeploymentLog(t, path)
			}
		})
	}
}

func TestBuildOutputProcess(t *testing.T) {
	mode := os.Getenv("BUILD_OUTPUT_HELPER")
	if mode == "" {
		return
	}
	if mode == "write-error" {
		fmt.Fprintln(os.Stdout, "visible output")
		os.Exit(0)
	}
	if mode == "append" {
		file, err := os.OpenFile(os.Getenv("BUILD_LOG_PATH"), os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			os.Exit(2)
		}
		for index := 0; index < 16; index++ {
			if _, err := WriteDeploymentLog(file, "concurrent process output\n"); err != nil {
				os.Exit(3)
			}
		}
		file.Close()
		os.Exit(0)
	}
	fmt.Fprintln(os.Stdout, "No start command detected")
	fmt.Fprint(os.Stdout, "credential=TOP")
	fmt.Fprintln(os.Stdout, "SECRET")
	if mode == "refined" {
		for index := 0; index < 32; index++ {
			fmt.Fprint(os.Stdout, strings.Repeat("x", buildLineMaxBytes))
		}
		fmt.Fprintln(os.Stdout)
	} else {
		line := strings.Repeat("x", buildLineMaxBytes-1) + "\n"
		for index := 0; index < 11*16; index++ {
			fmt.Fprint(os.Stdout, line)
		}
	}
	fmt.Fprintln(os.Stdout, "final stdout diagnostic")
	for index := 0; index < 32; index++ {
		fmt.Fprint(os.Stderr, strings.Repeat("y", buildLineMaxBytes))
	}
	fmt.Fprintln(os.Stderr, "\nfinal stderr diagnostic")
	if mode == "failure" {
		os.Exit(17)
	}
	os.Exit(0)
}

func TestBuildRunnerReportsStorageErrors(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := RunWithRefinedLogCtx(context.Background(), time.Second, filepath.Join(t.TempDir(), "missing", "build.log"), nil, executable)
	if result == nil || err == nil {
		t.Fatal("log-open failure must return an error and a non-nil result")
	}
	result, err = RunWithRefinedLogAndEnvCtx(context.Background(), 5*time.Second, []string{"BUILD_OUTPUT_HELPER=write-error"}, "/dev/full", nil, nil, executable, "-test.run=^TestBuildOutputProcess$")
	if result == nil || !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("disk-full error lost: %v", err)
	}
}
