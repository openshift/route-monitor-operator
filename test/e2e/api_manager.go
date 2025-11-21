//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// api_manager.go - Manages the RHOBS Synthetics API server lifecycle for E2E tests
//
// This manager builds and runs the RHOBS Synthetics API as a local binary.
// The API uses file-based local storage (no etcd/K8s required) and listens on port 8080.
//
// Key responsibilities:
//   - Build the API binary from source
//   - Start the API with local storage configuration
//   - Wait for the API to become ready (health check)
//   - Provide URL for test access
//   - Clean up resources on stop

// RealAPIManager manages the lifecycle of the actual RHOBS Synthetics API server
type RealAPIManager struct {
	*ProcessManager
	apiURL  string
	port    int
	dataDir string
}

// NewRealAPIManager creates a new manager for the real API server
func NewRealAPIManager() *RealAPIManager {
	return &RealAPIManager{
		ProcessManager: NewProcessManager(
			"RHOBS Synthetics API",
			"RHOBS_SYNTHETICS_API_PATH",
			"rhobs-synthetics-api",
		),
		port:    8080,
		dataDir: filepath.Join(os.TempDir(), "rhobs-synthetics-api-test-data"),
	}
}

// Start builds and starts the real RHOBS Synthetics API server
func (m *RealAPIManager) Start() error {
	if m.IsStarted() {
		return fmt.Errorf("API server already started")
	}

	// Build the API server first
	if err := m.BuildBinary("rhobs-synthetics-api", "./cmd/api"); err != nil {
		return fmt.Errorf("failed to build API: %w", err)
	}

	// Clean up old data directory to ensure a fresh start
	if _, err := os.Stat(m.dataDir); err == nil {
		if err := os.RemoveAll(m.dataDir); err != nil {
			return fmt.Errorf("failed to remove old data directory: %w", err)
		}
	}

	// Create fresh data directory
	if err := os.MkdirAll(m.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Find an available port
	if err := m.findAvailablePort(); err != nil {
		return fmt.Errorf("failed to find available port: %w", err)
	}

	m.apiURL = fmt.Sprintf("http://localhost:%d", m.port)

	// Start the API server
	if err := m.startAPI(); err != nil {
		return fmt.Errorf("failed to start API: %w", err)
	}

	// Wait for API to be ready
	if err := m.waitForAPI(); err != nil {
		_ = m.Stop()
		return fmt.Errorf("API failed to become ready: %w", err)
	}

	return nil
}

// GetURL returns the API server URL
func (m *RealAPIManager) GetURL() string {
	return m.apiURL
}

// findAvailablePort finds an available port for the API server
func (m *RealAPIManager) findAvailablePort() error {
	for port := m.port; port < m.port+100; port++ {
		if m.isPortAvailable(port) {
			m.port = port
			return nil
		}
	}
	return fmt.Errorf("no available port found in range %d-%d", m.port, m.port+100)
}

// isPortAvailable checks if a port is available
func (m *RealAPIManager) isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// startAPI starts the API server binary
func (m *RealAPIManager) startAPI() error {
	sourcePath, err := m.GetSourcePath()
	if err != nil {
		return err
	}

	binaryPath := filepath.Join(sourcePath, "rhobs-synthetics-api")

	// API uses Cobra commands, needs "start" subcommand with flags
	cmd := exec.CommandContext(m.Context(), binaryPath, "start",
		"--database-engine", "local",
		"--data-dir", m.dataDir,
		"--port", fmt.Sprintf("%d", m.port),
		"--log-level", "info",
	)
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

	// Stream logs in background
	go m.StreamOutput(stdout, "[api] ", nil)
	go m.StreamOutput(stderr, "[api] ", nil)

	if err := m.StartProcess(cmd); err != nil {
		return err
	}

	fmt.Printf("✅ API server starting on port %d\n", m.port)
	return nil
}

// waitForAPI waits for the API server to become ready
func (m *RealAPIManager) waitForAPI() error {
	fmt.Println("Waiting for API server to become ready...")

	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	maxAttempts := 30
	for i := 0; i < maxAttempts; i++ {
		resp, err := client.Get(m.apiURL + "/readyz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			fmt.Printf("✅ API server is ready at %s\n", m.apiURL)
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("API server did not become ready after %d seconds", maxAttempts/2)
}

// ClearAllProbes deletes all probes from the API
func (m *RealAPIManager) ClearAllProbes() error {
	client := &http.Client{Timeout: 5 * time.Second}

	// Get all probes
	resp, err := client.Get(m.apiURL + "/probes")
	if err != nil {
		return fmt.Errorf("failed to list probes: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to list probes: status %d", resp.StatusCode)
	}

	var probes []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&probes); err != nil {
		return fmt.Errorf("failed to decode probes: %w", err)
	}

	// Delete each probe
	for _, probe := range probes {
		probeID, ok := probe["id"].(string)
		if !ok {
			continue
		}

		req, err := http.NewRequest(http.MethodDelete, m.apiURL+"/probes/"+probeID, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
	}

	return nil
}
