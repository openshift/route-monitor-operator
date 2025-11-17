//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// process_manager.go - Shared base class for managing external processes (DRY pattern)
//
// This provides common functionality for both API and Agent managers:
//   - Finding source code paths (env vars or sibling directories)
//   - Building Go binaries (make or go build)
//   - Starting/stopping processes with graceful shutdown
//   - Streaming process output with optional filtering
//
// This follows the DRY (Don't Repeat Yourself) principle - common process
// management logic is written once and shared by multiple managers.

// ProcessManager provides common functionality for managing external processes
type ProcessManager struct {
	name        string
	cmd         *exec.Cmd
	sourcePath  string
	envVarName  string
	repoName    string
	stopChan    chan struct{}
	started     bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewProcessManager creates a base process manager
func NewProcessManager(name, envVarName, repoName string) *ProcessManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Get path from environment variable
	sourcePath := os.Getenv(envVarName)

	return &ProcessManager{
		name:       name,
		sourcePath: sourcePath,
		envVarName: envVarName,
		repoName:   repoName,
		stopChan:   make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// GetSourcePath returns the source code path, checking for existence
func (pm *ProcessManager) GetSourcePath() (string, error) {
	if pm.sourcePath == "" {
		return "", fmt.Errorf(`%s path not set.

To run E2E tests, you need local copies of the RHOBS repos.
Set the environment variable:

  export %s=/path/to/%s

Or clone it to a sibling directory:

  cd .. && git clone https://github.com/rhobs/%s.git
  cd route-monitor-operator
  export %s=$(cd ../%s && pwd)
  make test-e2e-full`,
			pm.name, pm.envVarName, pm.repoName, pm.repoName, pm.envVarName, pm.repoName)
	}

	if _, err := os.Stat(pm.sourcePath); os.IsNotExist(err) {
		return "", fmt.Errorf("%s path does not exist: %s", pm.name, pm.sourcePath)
	}

	return pm.sourcePath, nil
}

// BuildBinary builds a Go binary using make or go build
func (pm *ProcessManager) BuildBinary(binaryName, buildTarget string) error {
	fmt.Printf("Building %s...\n", binaryName)

	sourcePath, err := pm.GetSourcePath()
	if err != nil {
		return err
	}

	// Check if Makefile exists (use make build) or fallback to go build
	makefilePath := filepath.Join(sourcePath, "Makefile")
	if _, err := os.Stat(makefilePath); err == nil {
		// Local repo with Makefile - use make build
		cleanCmd := exec.CommandContext(pm.ctx, "make", "clean")
		cleanCmd.Dir = sourcePath
		_ = cleanCmd.Run() // Ignore errors from clean

		cmd := exec.CommandContext(pm.ctx, "make", "build")
		cmd.Dir = sourcePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to build %s: %w", binaryName, err)
		}
	} else {
		// No Makefile - use go build directly
		cmd := exec.CommandContext(pm.ctx, "go", "build", "-o", binaryName, buildTarget)
		cmd.Dir = sourcePath
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to build %s: %w", binaryName, err)
		}
	}

	fmt.Printf("✅ %s built successfully\n", binaryName)
	return nil
}

// StartProcess starts the managed process
func (pm *ProcessManager) StartProcess(cmd *exec.Cmd) error {
	pm.cmd = cmd

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", pm.name, err)
	}

	pm.started = true

	// Monitor for context cancellation
	go func() {
		<-pm.ctx.Done()
		if pm.cmd != nil && pm.cmd.Process != nil {
			_ = pm.cmd.Process.Signal(syscall.SIGTERM)
		}
	}()

	return nil
}

// Stop gracefully stops the managed process
func (pm *ProcessManager) Stop() error {
	if !pm.started {
		return nil
	}

	pm.cancel() // Cancel context

	if pm.cmd != nil && pm.cmd.Process != nil {
		// Try graceful shutdown first
		if err := pm.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to send SIGTERM to %s: %w", pm.name, err)
		}

		// Wait for graceful shutdown with timeout
		done := make(chan error)
		go func() {
			done <- pm.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill if still running
			_ = pm.cmd.Process.Kill()
			<-done
		}
	}

	close(pm.stopChan)
	pm.started = false
	return nil
}

// StreamOutput streams process output line by line
func (pm *ProcessManager) StreamOutput(reader io.Reader, prefix string, filter func(string) bool) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if filter == nil || filter(line) {
			fmt.Printf("%s%s\n", prefix, line)
		}
	}
}

// IsStarted returns whether the process is started
func (pm *ProcessManager) IsStarted() bool {
	return pm.started
}

// Context returns the process context
func (pm *ProcessManager) Context() context.Context {
	return pm.ctx
}

