//go:build e2e
// +build e2e

package e2e

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// AgentManager manages the lifecycle of the RHOBS Synthetics Agent
type AgentManager struct {
	cmd         *exec.Cmd
	agentPath   string
	configPath  string
	apiURL      string
	stopChan    chan struct{}
	started     bool
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewAgentManager creates a new manager for the agent
func NewAgentManager(apiURL string) *AgentManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Get agent path - must be set via environment variable
	// The agent must be available as source code to build the binary
	agentPath := os.Getenv("RHOBS_SYNTHETICS_AGENT_PATH")

	return &AgentManager{
		agentPath: agentPath,
		apiURL:    apiURL,
		stopChan:  make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start builds and starts the agent
func (m *AgentManager) Start() error {
	if m.started {
		return fmt.Errorf("agent already started")
	}

	// Build the agent binary
	if err := m.buildAgent(); err != nil {
		return fmt.Errorf("failed to build agent: %w", err)
	}

	// Create config file
	if err := m.createConfig(); err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// Start the agent
	if err := m.startAgent(); err != nil {
		return fmt.Errorf("failed to start agent: %w", err)
	}

	m.started = true
	return nil
}

// buildAgent builds the agent binary
func (m *AgentManager) buildAgent() error {
	fmt.Println("Building rhobs-synthetics-agent...")

	// Check if agent path exists
	if m.agentPath == "" {
		return fmt.Errorf(`RHOBS Synthetics Agent path not set.

To run E2E tests, you need local copies of the RHOBS repos.
Set the environment variable:

  export RHOBS_SYNTHETICS_AGENT_PATH=/path/to/rhobs-synthetics-agent

Or clone it to a sibling directory:

  cd .. && git clone https://github.com/rhobs/rhobs-synthetics-agent.git
  cd route-monitor-operator
  export RHOBS_SYNTHETICS_AGENT_PATH=$(cd ../rhobs-synthetics-agent && pwd)
  make test-e2e-full`)
	}
	
	if _, err := os.Stat(m.agentPath); os.IsNotExist(err) {
		return fmt.Errorf("agent path does not exist: %s", m.agentPath)
	}

	// First, tidy dependencies
	fmt.Println("Tidying agent dependencies...")
	tidyCmd := exec.CommandContext(m.ctx, "go", "mod", "tidy")
	tidyCmd.Dir = m.agentPath
	if output, err := tidyCmd.CombinedOutput(); err != nil {
		fmt.Printf("go mod tidy output: %s\n", string(output))
		return fmt.Errorf("failed to tidy dependencies: %w", err)
	}

	// Build the agent
	buildCmd := exec.CommandContext(m.ctx, "go", "build", "-o", "agent", "./cmd/agent")
	buildCmd.Dir = m.agentPath
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build agent: %w", err)
	}

	fmt.Println("rhobs-synthetics-agent built successfully.")
	return nil
}

// createConfig creates the agent configuration file
func (m *AgentManager) createConfig() error {
	configContent := fmt.Sprintf(`---
api_urls:
  - %s/probes
label_selector: "app=rhobs-synthetics-probe"
polling_interval: 2s
log_level: debug
log_format: text
`, m.apiURL)

	m.configPath = filepath.Join(os.TempDir(), "agent-config.yaml")
	if err := os.WriteFile(m.configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// startAgent starts the agent process
func (m *AgentManager) startAgent() error {
	binaryPath := filepath.Join(m.agentPath, "agent")
	
	// Try with -config (single dash) instead of --config (double dash)
	m.cmd = exec.CommandContext(m.ctx, binaryPath, "-config", m.configPath)
	m.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Capture stdout and stderr
	stdout, err := m.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := m.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start the process
	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agent process: %w", err)
	}

	// Stream logs in background (filter out noise)
	go m.streamLogs(stdout, "STDOUT")
	go m.streamLogs(stderr, "STDERR")

	// Wait a bit for agent to start
	time.Sleep(2 * time.Second)

	// Check if process is still running
	if m.cmd.Process == nil {
		return fmt.Errorf("agent process failed to start")
	}

	return nil
}

// streamLogs streams logs from the agent
func (m *AgentManager) streamLogs(reader io.Reader, prefix string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		// Filter out known noise (flag/command errors, usage text)
		if strings.Contains(line, "unknown flag") || 
		   strings.Contains(line, "unknown command") ||
		   strings.Contains(line, "Usage:") ||
		   strings.Contains(line, "Flags:") {
			continue // Skip startup noise
		}
		// Only print if it contains useful information
		if strings.Contains(line, "error") || strings.Contains(line, "ERROR") ||
			strings.Contains(line, "fetched") || strings.Contains(line, "probe") {
			fmt.Printf("[Agent %s] %s\n", prefix, line)
		}
	}
}

// Stop stops the agent
func (m *AgentManager) Stop() error {
	if !m.started {
		return nil
	}

	// Cancel context to signal shutdown
	m.cancel()

	// Send SIGTERM to process group
	if m.cmd != nil && m.cmd.Process != nil {
		pgid, err := syscall.Getpgid(m.cmd.Process.Pid)
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
		}

		// Wait for process to exit with timeout
		done := make(chan error, 1)
		go func() {
			done <- m.cmd.Wait()
		}()

		select {
		case <-done:
			// Process exited
		case <-time.After(5 * time.Second):
			// Force kill if still running
			if m.cmd.Process != nil {
				_ = m.cmd.Process.Kill()
			}
		}
	}

	// Clean up config file
	if m.configPath != "" {
		_ = os.Remove(m.configPath)
	}

	m.started = false
	close(m.stopChan)

	return nil
}

// isPortAvailable checks if a port is available
func isPortAvailable(port int) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

