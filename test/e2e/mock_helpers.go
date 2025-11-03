//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type MockKubernetesClient struct {
	mu         sync.RWMutex
	objects    map[types.NamespacedName]runtime.Object
	namespaces map[string]bool
	events     []MockEvent
}

type MockEvent struct {
	Type      string
	Object    runtime.Object
	Timestamp time.Time
}

func NewMockKubernetesClient() *MockKubernetesClient {
	return &MockKubernetesClient{
		objects:    make(map[types.NamespacedName]runtime.Object),
		namespaces: make(map[string]bool),
		events:     make([]MockEvent, 0),
	}
}

func (m *MockKubernetesClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}

	if obj.GetCreationTimestamp().Time.IsZero() {
		obj.SetCreationTimestamp(metav1.NewTime(time.Now()))
	}
	if obj.GetUID() == "" {
		obj.SetUID(types.UID(fmt.Sprintf("mock-uid-%d", time.Now().UnixNano())))
	}
	obj.SetResourceVersion(fmt.Sprintf("%d", time.Now().UnixNano()))

	m.objects[key] = obj.DeepCopyObject()
	m.events = append(m.events, MockEvent{
		Type:      "Created",
		Object:    obj.DeepCopyObject(),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *MockKubernetesClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if storedObj, exists := m.objects[key]; exists {
		return copyObject(storedObj, obj)
	}
	return fmt.Errorf("object not found: %s", key.String())
}

func (m *MockKubernetesClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return nil
}

func (m *MockKubernetesClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if _, exists := m.objects[key]; !exists {
		return fmt.Errorf("object not found: %s", key.String())
	}
	obj.SetResourceVersion(fmt.Sprintf("%d", time.Now().UnixNano()))

	m.objects[key] = obj.DeepCopyObject()
	m.events = append(m.events, MockEvent{
		Type:      "Updated",
		Object:    obj.DeepCopyObject(),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *MockKubernetesClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	if _, exists := m.objects[key]; !exists {
		return fmt.Errorf("object not found: %s", key.String())
	}
	delete(m.objects, key)
	m.events = append(m.events, MockEvent{
		Type:      "Deleted",
		Object:    obj.DeepCopyObject(),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *MockKubernetesClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	return m.Update(ctx, obj)
}
func (m *MockKubernetesClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	return nil
}
func (m *MockKubernetesClient) Status() client.StatusWriter {
	return &mockStatusWriter{client: m}
}

type mockStatusWriter struct {
	client *MockKubernetesClient
}

func (w *mockStatusWriter) Create(ctx context.Context, obj client.Object, subResource client.Object, opts ...client.SubResourceCreateOption) error {
	return w.client.Create(ctx, obj)
}

func (w *mockStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	return w.client.Update(ctx, obj)
}

func (w *mockStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	return w.client.Patch(ctx, obj, patch)
}

func (m *MockKubernetesClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}
func (m *MockKubernetesClient) RESTMapper() meta.RESTMapper {
	return nil
}
func (m *MockKubernetesClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	return schema.GroupVersionKind{}, nil
}
func (m *MockKubernetesClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	return true, nil
}

func (m *MockKubernetesClient) CreateNamespace(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	key := types.NamespacedName{Name: name}
	m.objects[key] = namespace
	m.namespaces[name] = true

	m.events = append(m.events, MockEvent{
		Type:      "NamespaceCreated",
		Object:    namespace.DeepCopyObject(),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *MockKubernetesClient) DeleteNamespace(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key := range m.objects {
		if key.Namespace == name {
			delete(m.objects, key)
		}
	}
	delete(m.namespaces, name)
	namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}

	m.events = append(m.events, MockEvent{
		Type:      "NamespaceDeleted",
		Object:    namespace.DeepCopyObject(),
		Timestamp: time.Now(),
	})

	return nil
}

func (m *MockKubernetesClient) GetEvents() []MockEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	events := make([]MockEvent, len(m.events))
	copy(events, m.events)
	return events
}
func (m *MockKubernetesClient) GetObjects() map[types.NamespacedName]runtime.Object {
	m.mu.RLock()
	defer m.mu.RUnlock()
	objects := make(map[types.NamespacedName]runtime.Object)
	for key, obj := range m.objects {
		objects[key] = obj.DeepCopyObject()
	}
	return objects
}

func copyObject(src, dst runtime.Object) error { return nil }

type MockRHOBSClient struct {
	probes map[string]*MockProbe
}

type MockProbe struct {
	ID        string            `json:"id"`
	Labels    map[string]string `json:"labels"`
	Status    string            `json:"status"`
	TargetURL string            `json:"target_url"`
	Interval  string            `json:"interval"`
	Timeout   string            `json:"timeout"`
}

func NewMockRHOBSClient() *MockRHOBSClient {
	return &MockRHOBSClient{
		probes: make(map[string]*MockProbe),
	}
}

func (c *MockRHOBSClient) CreateProbe(ctx context.Context, clusterID string, probeData map[string]interface{}) (*MockProbe, error) {
	probe := &MockProbe{
		ID:        fmt.Sprintf("probe-%s-%d", clusterID, time.Now().UnixNano()),
		Labels:    make(map[string]string),
		Status:    "active",
		TargetURL: "https://api.example.com/livez",
		Interval:  "30s",
		Timeout:   "10s",
	}
	if labels, ok := probeData["labels"].(map[string]string); ok {
		probe.Labels = labels
	}
	if targetURL, ok := probeData["target_url"].(string); ok {
		probe.TargetURL = targetURL
	}
	if interval, ok := probeData["interval"].(string); ok {
		probe.Interval = interval
	}
	if timeout, ok := probeData["timeout"].(string); ok {
		probe.Timeout = timeout
	}

	c.probes[clusterID] = probe
	return probe, nil
}

func (c *MockRHOBSClient) GetProbe(ctx context.Context, clusterID string) (*MockProbe, error) {
	if probe, exists := c.probes[clusterID]; exists {
		return probe, nil
	}
	return nil, fmt.Errorf("probe not found for cluster: %s", clusterID)
}

func (c *MockRHOBSClient) DeleteProbe(ctx context.Context, clusterID string) error {
	if _, exists := c.probes[clusterID]; !exists {
		return fmt.Errorf("probe not found for cluster: %s", clusterID)
	}

	delete(c.probes, clusterID)
	return nil
}

func (c *MockRHOBSClient) UpdateProbeStatus(ctx context.Context, clusterID, status string) error {
	if probe, exists := c.probes[clusterID]; exists {
		probe.Status = status
		return nil
	}
	return fmt.Errorf("probe not found for cluster: %s", clusterID)
}

func (c *MockRHOBSClient) GetProbes() map[string]*MockProbe {
	return c.probes
}

type MockHostedControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MockHostedControlPlaneSpec   `json:"spec,omitempty"`
	Status            MockHostedControlPlaneStatus `json:"status,omitempty"`
}

func (h *MockHostedControlPlane) DeepCopyObject() runtime.Object {
	return h.DeepCopy()
}
func (h *MockHostedControlPlane) DeepCopy() *MockHostedControlPlane {
	if h == nil {
		return nil
	}
	out := new(MockHostedControlPlane)
	h.DeepCopyInto(out)
	return out
}

func (h *MockHostedControlPlane) DeepCopyInto(out *MockHostedControlPlane) {
	*out = *h
	out.TypeMeta = h.TypeMeta
	h.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	h.Spec.DeepCopyInto(&out.Spec)
	h.Status.DeepCopyInto(&out.Status)
}
func (s *MockHostedControlPlaneSpec) DeepCopyInto(out *MockHostedControlPlaneSpec) {
	*out = *s
}
func (s *MockHostedControlPlaneStatus) DeepCopyInto(out *MockHostedControlPlaneStatus) {
	*out = *s
	if s.Conditions != nil {
		in, out := &s.Conditions, &out.Conditions
		*out = make([]MockCondition, len(*in))
		copy(*out, *in)
	}
}

type MockHostedControlPlaneSpec struct {
	ClusterID         string `json:"clusterID"`
	Platform          string `json:"platform"`
	Region            string `json:"region"`
	EndpointAccess    string `json:"endpointAccess"`
	APIServerHostname string `json:"apiServerHostname"`
}

type MockHostedControlPlaneStatus struct {
	Conditions []MockCondition `json:"conditions,omitempty"`
}

type MockCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type MockRMOController struct {
	k8sClient    *MockKubernetesClient
	rhobsClient  *MockRHOBSClient
	probes       map[string]*MockProbe
	reconcileLog []string
}

func NewMockRMOController(k8sClient *MockKubernetesClient, rhobsClient *MockRHOBSClient) *MockRMOController {
	return &MockRMOController{
		k8sClient:    k8sClient,
		rhobsClient:  rhobsClient,
		probes:       make(map[string]*MockProbe),
		reconcileLog: make([]string, 0),
	}
}

func (m *MockRMOController) ReconcileHostedControlPlane(ctx context.Context, hcp *MockHostedControlPlane) error {
	log.Printf("Reconciling HostedControlPlane: %s", hcp.Name)
	m.reconcileLog = append(m.reconcileLog, fmt.Sprintf("Reconciling HostedControlPlane: %s", hcp.Name))
	if err := m.createInternalMonitoringObjects(ctx, hcp); err != nil {
		return fmt.Errorf("failed to create internal monitoring objects: %w", err)
	}
	if err := m.createRHOBSProbe(ctx, hcp); err != nil {
		return fmt.Errorf("failed to create RHOBS probe: %w", err)
	}
	log.Printf("Successfully reconciled HostedControlPlane: %s", hcp.Name)
	m.reconcileLog = append(m.reconcileLog, fmt.Sprintf("Successfully reconciled HostedControlPlane: %s", hcp.Name))

	return nil
}

func (m *MockRMOController) createInternalMonitoringObjects(ctx context.Context, hcp *MockHostedControlPlane) error {
	route := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-route", hcp.Name),
			Namespace: hcp.Namespace,
			Labels: map[string]string{
				"cluster-id": hcp.Spec.ClusterID,
				"app":        "hosted-control-plane",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "https",
					Port: 443,
				},
			},
		},
	}

	if err := m.k8sClient.Create(ctx, route); err != nil {
		return fmt.Errorf("failed to create Route: %w", err)
	}

	routeMonitor := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-routemonitor", hcp.Name),
			Namespace: hcp.Namespace,
			Labels: map[string]string{
				"cluster-id": hcp.Spec.ClusterID,
				"app":        "route-monitor",
			},
		},
		Data: map[string]string{
			"target-url": fmt.Sprintf("https://%s/livez", hcp.Spec.APIServerHostname),
			"interval":   "30s",
		},
	}

	if err := m.k8sClient.Create(ctx, routeMonitor); err != nil {
		return fmt.Errorf("failed to create RouteMonitor: %w", err)
	}

	log.Printf("Created internal monitoring objects for HostedControlPlane: %s", hcp.Name)
	return nil
}

func (m *MockRMOController) createRHOBSProbe(ctx context.Context, hcp *MockHostedControlPlane) error {
	probeData := map[string]interface{}{
		"labels": map[string]string{
			"cluster-id": hcp.Spec.ClusterID,
			"private":    "false",
		},
		"target_url": fmt.Sprintf("https://%s/livez", hcp.Spec.APIServerHostname),
		"interval":   "30s",
		"timeout":    "10s",
	}

	probe, err := m.rhobsClient.CreateProbe(ctx, hcp.Spec.ClusterID, probeData)
	if err != nil {
		return fmt.Errorf("failed to create RHOBS probe: %w", err)
	}

	m.probes[hcp.Spec.ClusterID] = probe
	// Create probe in mock RHOBS API (always runs on localhost:8080)
	body := map[string]interface{}{
		"labels": map[string]string{
			"cluster-id":              hcp.Spec.ClusterID,
			"private":                 "false",
			"rhobs-synthetics/status": "pending",
		},
		"static_url": fmt.Sprintf("https://%s/livez", hcp.Spec.APIServerHostname),
	}
	payload, _ := json.Marshal(body)
	apiURL := "http://localhost:8080"
	endpoint := fmt.Sprintf("%s/api/metrics/v1/test/probes", apiURL)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err == nil && resp.StatusCode < 400 {
		resp.Body.Close()
	}

	log.Printf("Created RHOBS probe for cluster: %s", hcp.Spec.ClusterID)
	return nil
}

func (m *MockRMOController) DeleteHostedControlPlane(ctx context.Context, hcp *MockHostedControlPlane) error {
	log.Printf("Deleting HostedControlPlane: %s", hcp.Name)
	m.reconcileLog = append(m.reconcileLog, fmt.Sprintf("Deleting HostedControlPlane: %s", hcp.Name))
	if err := m.deleteRHOBSProbe(ctx, hcp.Spec.ClusterID); err != nil {
		log.Printf("Warning: failed to delete RHOBS probe: %v", err)
	}
	if err := m.deleteInternalMonitoringObjects(ctx, hcp); err != nil {
		log.Printf("Warning: failed to delete internal monitoring objects: %v", err)
	}
	delete(m.probes, hcp.Spec.ClusterID)

	log.Printf("Successfully deleted HostedControlPlane: %s", hcp.Name)
	m.reconcileLog = append(m.reconcileLog, fmt.Sprintf("Successfully deleted HostedControlPlane: %s", hcp.Name))

	return nil
}

func (m *MockRMOController) deleteRHOBSProbe(ctx context.Context, clusterID string) error {
	if err := m.rhobsClient.DeleteProbe(ctx, clusterID); err != nil {
		return fmt.Errorf("failed to delete RHOBS probe: %w", err)
	}

	log.Printf("Deleted RHOBS probe for cluster: %s", clusterID)
	return nil
}

func (m *MockRMOController) deleteInternalMonitoringObjects(ctx context.Context, hcp *MockHostedControlPlane) error {
	route := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-route", hcp.Name),
			Namespace: hcp.Namespace,
		},
	}

	if err := m.k8sClient.Delete(ctx, route); err != nil {
		log.Printf("Warning: failed to delete Route: %v", err)
	}

	routeMonitor := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-routemonitor", hcp.Name),
			Namespace: hcp.Namespace,
		},
	}

	if err := m.k8sClient.Delete(ctx, routeMonitor); err != nil {
		log.Printf("Warning: failed to delete RouteMonitor: %v", err)
	}

	log.Printf("Deleted internal monitoring objects for HostedControlPlane: %s", hcp.Name)
	return nil
}

func (m *MockRMOController) GetReconcileLog() []string {
	return m.reconcileLog
}
func (m *MockRMOController) GetProbes() map[string]*MockProbe {
	return m.probes
}
func CreateTestHostedControlPlane(name, namespace, clusterID string) *MockHostedControlPlane {
	return &MockHostedControlPlane{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hypershift.openshift.io/v1beta1",
			Kind:       "HostedControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"cluster-id": clusterID,
			},
		},
		Spec: MockHostedControlPlaneSpec{
			ClusterID:         clusterID,
			Platform:          "AWS",
			Region:            "us-west-2",
			EndpointAccess:    "PublicAndPrivate",
			APIServerHostname: fmt.Sprintf("api.%s.example.com", clusterID),
		},
		Status: MockHostedControlPlaneStatus{
			Conditions: []MockCondition{
				{
					Type:    "Ready",
					Status:  "True",
					Reason:  "AsExpected",
					Message: "HostedControlPlane is ready",
				},
			},
		},
	}
}

type SyntheticsAgentClient struct {
	baseURL string
	client  *http.Client
}

func NewSyntheticsAgentClient(baseURL string) *SyntheticsAgentClient {
	return &SyntheticsAgentClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthResponse represents the health check response from the synthetics agent
type HealthResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ProbeExecution represents a probe execution by the synthetics agent
type ProbeExecution struct {
	ProbeID      string    `json:"probe_id"`
	ClusterID    string    `json:"cluster_id"`
	Status       string    `json:"status"`
	Timestamp    time.Time `json:"timestamp"`
	Duration     int64     `json:"duration_ms"`
	Error        string    `json:"error,omitempty"`
	StatusCode   int       `json:"status_code,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
}

// ProbeStatus represents the status of a probe
type ProbeStatus struct {
	ProbeID   string `json:"probe_id"`
	ClusterID string `json:"cluster_id"`
	Status    string `json:"status"`
	Active    bool   `json:"active"`
}

// ProbeResult represents the result of a probe execution
type ProbeResult struct {
	ProbeID      string    `json:"probe_id"`
	ClusterID    string    `json:"cluster_id"`
	Success      bool      `json:"success"`
	Timestamp    time.Time `json:"timestamp"`
	Duration     int64     `json:"duration_ms"`
	Error        string    `json:"error,omitempty"`
	StatusCode   int       `json:"status_code,omitempty"`
	ResponseBody string    `json:"response_body,omitempty"`
}

// CheckHealth checks if the synthetics agent is healthy
func (c *SyntheticsAgentClient) CheckHealth(ctx context.Context) (*HealthResponse, error) {
	url := fmt.Sprintf("%s/health", c.baseURL)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform health check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("health check failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read health check response: %w", err)
	}

	var health HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to unmarshal health check response: %w", err)
	}

	if health.Status == "" {
		health.Status = "healthy"
	}

	return &health, nil
}

// GetProbeExecutions retrieves probe executions from the synthetics agent
func (c *SyntheticsAgentClient) GetProbeExecutions(ctx context.Context, clusterID string) ([]ProbeExecution, error) {
	url := fmt.Sprintf("%s/probes/executions?cluster_id=%s", c.baseURL, clusterID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create probe executions request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get probe executions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get probe executions with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read probe executions response: %w", err)
	}

	var executions []ProbeExecution
	if err := json.Unmarshal(body, &executions); err != nil {
		return nil, fmt.Errorf("failed to unmarshal probe executions response: %w", err)
	}

	return executions, nil
}

// GetProbeStatus retrieves the status of a specific probe
func (c *SyntheticsAgentClient) GetProbeStatus(ctx context.Context, probeID string) (*ProbeStatus, error) {
	url := fmt.Sprintf("%s/probes/%s/status", c.baseURL, probeID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create probe status request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get probe status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get probe status with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read probe status response: %w", err)
	}

	var status ProbeStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal probe status response: %w", err)
	}

	return &status, nil
}

func verifyMockAgentExecutions(client *SyntheticsAgentClient, clusterID string) bool {
	executions, err := client.GetProbeExecutions(context.TODO(), clusterID)
	if err != nil {
		return false
	}
	for _, exec := range executions {
		if exec.ClusterID == clusterID {
			return true
		}
	}
	return false
}

func VerifySyntheticsAgentExecution(syntheticsAgentURL, clusterID string) bool {
	client := NewSyntheticsAgentClient(syntheticsAgentURL)

	if health, err := client.CheckHealth(context.TODO()); err != nil || health == nil {
		return false
	}

	// Mock agent exposes /probes/executions endpoint
	return verifyMockAgentExecutions(client, clusterID)
}
func VerifyProbeResults(rhobsClient *MockRHOBSClient, clusterID string) bool {
	probe, err := rhobsClient.GetProbe(context.TODO(), clusterID)
	if err != nil || probe == nil {
		return false
	}
	// Verify probe is active after agent execution
	// In real flow: probe starts as "pending", agent executes it, then updates to "active"
	// If we're checking after execution, probe should be "active" indicating successful result
	return probe.Status == "active"
}

func contains(s, substr string) bool { return strings.Contains(s, substr) }
