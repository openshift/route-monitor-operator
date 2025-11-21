//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// agent_manager.go - Manages the RHOBS Synthetics Agent lifecycle for E2E tests
//
// This manager builds and runs the RHOBS Synthetics Agent as a local binary.
// The agent connects to the API to fetch probes but cannot fully process them
// without a Kubernetes cluster (see full_integration_test.go for mock approach).
//
// Key responsibilities:
//   - Build the agent binary from source
//   - Create agent configuration (API URL, sync interval, metrics port)
//   - Start the agent process
//   - Filter verbose agent logs for test readability
//   - Gracefully stop the agent

// AgentManager manages the lifecycle of the RHOBS Synthetics Agent
type AgentManager struct {
	*ProcessManager
	configPath string
	apiURL     string
}

// NewAgentManager creates a new manager for the agent
func NewAgentManager(apiURL string) *AgentManager {
	return &AgentManager{
		ProcessManager: NewProcessManager(
			"RHOBS Synthetics Agent",
			"RHOBS_SYNTHETICS_AGENT_PATH",
			"rhobs-synthetics-agent",
		),
		apiURL: apiURL,
	}
}

// Start builds and starts the agent
func (m *AgentManager) Start() error {
	if m.IsStarted() {
		return fmt.Errorf("agent already started")
	}

	// Build the agent binary
	if err := m.BuildBinary("rhobs-synthetics-agent", "./cmd/agent"); err != nil {
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

	return nil
}

// createConfig creates the agent configuration file
func (m *AgentManager) createConfig() error {
	// Agent expects api_urls as an array of complete API URLs including /probes path
	apiURL := m.apiURL + "/probes"
	config := map[string]interface{}{
		"api_urls":      []string{apiURL},
		"sync_interval": "5s",
		"log_level":     "info",
		"metrics_port":  8081, // Use different port than API (8080)
	}

	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	m.configPath = filepath.Join(os.TempDir(), "agent-config.yaml")
	if err := os.WriteFile(m.configPath, configBytes, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("✅ Agent config created at %s\n", m.configPath)
	return nil
}

// startAgent starts the agent binary
func (m *AgentManager) startAgent() error {
	sourcePath, err := m.GetSourcePath()
	if err != nil {
		return err
	}

	binaryPath := filepath.Join(sourcePath, "rhobs-synthetics-agent")
	
	cmd := exec.CommandContext(m.Context(), binaryPath, "start", "--config", m.configPath)
	cmd.Dir = sourcePath

	// Capture stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	// Start streaming logs in background with filtering
	go m.StreamOutput(stdout, "[agent] ", m.filterAgentLogs)
	go m.StreamOutput(stderr, "[agent] ", m.filterAgentLogs)

	if err := m.StartProcess(cmd); err != nil {
		return err
	}

	fmt.Printf("✅ Agent started successfully\n")
	return nil
}

// filterAgentLogs filters verbose agent output
func (m *AgentManager) filterAgentLogs(line string) bool {
	// Filter out known verbose messages
	verbosePatterns := []string{
		"unknown flag:",
		"Error: unknown flag:",
		"Error: unknown command",
		"level=debug",
	}

	for _, pattern := range verbosePatterns {
		if strings.Contains(line, pattern) {
			return false
		}
	}

	// Only show important events
	importantPatterns := []string{
		"level=info",
		"level=warn",
		"level=error",
		"Fetched",
		"probe",
		"Synced",
		"Starting",
		"Listening",
	}

	for _, pattern := range importantPatterns {
		if strings.Contains(line, pattern) {
			return true
		}
	}

	return false
}
