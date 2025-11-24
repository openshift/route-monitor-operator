# Blackbox Exporter: Detailed insights

## Table of Contents

1. [Blackbox Exporter Overview](#blackbox-exporter-overview)
2. [How Blackbox Exporter Works](#how-blackbox-exporter-works)
3. [Probe Types and Modules](#probe-types-and-modules)
4. [Route Monitor Operator Integration](#route-monitor-operator-integration)
5. [BlackBoxExporter Package](#blackboxexporter-package)
6. [Deployment and Configuration](#deployment-and-configuration)
7. [Troubleshooting and Monitoring](#troubleshooting-and-monitoring)

---

## Blackbox Exporter Overview

### What is Blackbox Exporter?

Blackbox Exporter is a Prometheus exporter that allows probing of endpoints over HTTP, HTTPS, DNS, TCP and ICMP. It is used to perform black-box monitoring of endpoints and alert on the results. Unlike traditional monitoring agents that run on monitored systems, Blackbox Exporter performs external, end-to-end monitoring from the perspective of users. The route monitor operator is making sure that there is one deployment + service of the blackbox exporter. If it does not exist in openshift-monitoring, it creates one.

### Key Characteristics

- **Endpoint Agnostic**: Can monitor any network-accessible endpoint without requiring an agent on the target
- **Protocol Support**: HTTP, HTTPS, DNS, TCP, ICMP, gRPC protocols
- **Metrics Generation**: Produces Prometheus metrics about probe success/failure, response time, and other performance indicators
- **Modular Configuration**: Uses YAML configuration files to define probes with different behaviors and requirements
- **Lightweight**: Minimal resource overhead, efficient probe execution

### Why Blackbox Exporter for Route Monitoring?

In the context of the Route Monitor Operator:

- **Route Health Verification**: Confirms that OpenShift routes are actually accessible and responding correctly
- **External Perspective**: Monitors from outside the cluster, simulating real user experience
- **HTTP Status Verification**: Validates that endpoints return expected HTTP status codes (e.g., 2xx for success)
- **TLS Flexibility**: Can probe both secure (HTTPS) and self-signed certificate endpoints
- **Availability Metrics**: Provides uptime and response time metrics for SLA tracking

---

## How Blackbox Exporter Works

### Architecture and Components

```
┌────────────────────────────────────────────────────────────┐
│             Blackbox Exporter Container                    │
│                                                            │
│  ┌──────────────────┐                                      │
│  │  Configuration   │                                      │
│  │  (blackbox.yaml) │                                      │
│  └──────────────────┘                                      │
│           ↓                                                │
│  ┌──────────────────────────────────────┐                  │
│  │  Probe Module Engine                 │                  │
│  │  - HTTP/HTTPS Module                 │                  │
│  │  - TCP Module                        │                  │
│  │  - DNS Module                        │                  │
│  │  - ICMP Module                       │                  │
│  │  - gRPC Module                       │                  │
│  └──────────────────────────────────────┘                  │
│           ↓                                                │
│  ┌──────────────────────────────────────┐                  │
│  │  Metrics Collection & Formatting     │                  │
│  │  (Prometheus compatible metrics)     │                  │
│  └──────────────────────────────────────┘                  │
│           ↓                                                │
│  ┌──────────────────────────────────────┐                  │
│  │  HTTP Endpoint (:9115/metrics)       │                  │
│  │  Exposing collected metrics          │                  │
│  └──────────────────────────────────────┘                  │
└────────────────────────────────────────────────────────────┘
           ↓
   Prometheus Server
   (scrapes metrics)
           ↓
   Alertmanager (optional)
   Grafana Dashboards
```

### Probe Flow

1. **Probe Request**: Prometheus server or operator requests a probe on a specific target
   - Request includes target URL
   - Request includes module name (defines probe behavior)
   - Request includes optional probe labels

2. **Module Selection**: Blackbox Exporter selects the appropriate probe module from configuration
   - Module defines protocol (HTTP, TCP, DNS, etc.)
   - Module defines parameters (timeout, expected codes, etc.)

3. **Probe Execution**: The selected module executes the probe
   - Connects to the target endpoint
   - Collects response data and timing information
   - Performs configured checks and validations

4. **Metrics Generation**: Successful or failed probe results are converted to metrics
   - `probe_success`: 1 if probe succeeded, 0 if failed
   - `probe_duration_seconds`: Time taken to complete probe
   - `probe_http_status_code`: HTTP status code returned
   - Additional module-specific metrics

5. **Metrics Exposure**: Metrics are exposed via HTTP endpoint for scraping

### Metrics Exposed

All Blackbox Exporter metrics are prefixed with `probe_`:

**Common Metrics:**
```
probe_success                          # 1 if probe succeeded, 0 otherwise
probe_duration_seconds                 # Time taken to complete probe
probe_http_status_code                 # HTTP status code (HTTP module)
probe_http_content_length              # Response body content length
probe_http_version                     # HTTP version used (1.0, 1.1, 2.0)
probe_http_ssl                         # 1 if SSL/TLS used, 0 otherwise
probe_ssl_earliest_cert_expiry         # Unix timestamp of earliest certificate expiry
probe_ssl_last_chain_expiry_timestamp_seconds # Certificate expiry time
```

---

## Probe Types and Modules

### HTTP/HTTPS Probing

The most commonly used probe type in Route Monitor Operator for monitoring OpenShift routes.

#### HTTP Module Configuration

```yaml
modules:
  http_2xx:
    prober: http
    timeout: 15s
    http:
      preferred_ip_protocol: "ip4"     # Use IPv4
      valid_http_versions: ["HTTP/1.0", "HTTP/1.1", "HTTP/2.0"]
      valid_status_codes:              # Expected success codes
        - 200
        - 201
        - 202
        - 203
        - 204
        - 206
        - 301
        - 302
        - 303
        - 304
        - 305
        - 307
```

#### HTTPS with Self-Signed Certificates

```yaml
  insecure_http_2xx:
    prober: http
    timeout: 15s
    http:
      preferred_ip_protocol: "ip4"
      tls_config:
        insecure_skip_verify: true      # Skip TLS certificate validation
      valid_http_versions: ["HTTP/1.0", "HTTP/1.1", "HTTP/2.0"]
      valid_status_codes:
        - 200
        - 201
        - 202
        - 203
        - 204
        - 206
        - 301
        - 302
        - 303
        - 304
        - 305
        - 307
```

#### Key HTTP Configuration Options

| Option | Purpose | Example |
|--------|---------|---------|
| `timeout` | Maximum time to wait for probe completion | `15s` |
| `expected_status_codes` | Acceptable HTTP response codes | `[200, 301, 302]` |
| `insecure_skip_verify` | Skip TLS certificate validation | `true/false` |
| `preferred_ip_protocol` | IP version preference | `ip4` or `ip6` |
| `no_follow_redirects` | Don't follow HTTP redirects | `true/false` |
| `valid_http_versions` | Accepted HTTP versions | `["HTTP/1.1", "HTTP/2.0"]` |
| `method` | HTTP request method | `GET`, `POST`, `HEAD` |
| `headers` | Custom HTTP headers | Map of header key-value pairs |

### TCP Probing

Used for monitoring TCP port availability (not used in default Route Monitor setup but available):

```yaml
  tcp_connect:
    prober: tcp
    timeout: 10s
    tcp:
      preferred_ip_protocol: "ip4"
```

### DNS Probing

Used for monitoring DNS resolution and responses:

```yaml
  dns_a:
    prober: dns
    timeout: 10s
    dns:
      preferred_ip_protocol: "ip4"
      query_name: "example.com"
      query_type: "A"
```

### ICMP Probing

Used for monitoring ICMP ping responses:

```yaml
  icmp:
    prober: icmp
    timeout: 10s
    icmp:
      preferred_ip_protocol: "ip4"
```

---

## Route Monitor Operator Integration

### Architecture Integration

```
┌───────────────────────────────────────────────────┐
│     Route Monitor Operator                        │
│                                                   │
│  ┌────────────────────────────────────────────┐   │
│  │ Route Monitor Controller                   │   │
│  │ (Watches RouteMonitor resources)           │   │
│  └────────────────────────────────────────────┘   │
│           ↓                                       │
│  ┌────────────────────────────────────────────┐   │
│  │ Blackbox Exporter Manager (this package)   │   │
│  │ (Manages Blackbox Exporter lifecycle)      │   │
│  └────────────────────────────────────────────┘   │
│           ↓                                       │
│  ┌─────────────────────────────────────────────┐  │
│  │  Resources Created                          │  │
│  │  - Deployment (runs blackbox exporter)      │  │
│  │  - Service (exposes metrics endpoint)       │  │
│  │  - ConfigMap (probe configurations)         │  │
│  └─────────────────────────────────────────────┘  │
│           ↓                                       │
│  ┌─────────────────────────────────────────────┐  │
│  │  Service Monitor (for Prometheus)           │  │
│  │  (scrapes metrics from Blackbox Exporter)   │  │
│  └─────────────────────────────────────────────┘  │
│           ↓                                       │
└───────────────────────────────────────────────────┘
        ↓
  ┌─────────────────────┐
  │  Prometheus Server  │
  │ (stores metrics)    │
  └─────────────────────┘
        ↓
  ┌──────────────────────┐
  │ Alertmanager         │
  │ (fires alerts)       │
  └──────────────────────┘
```

---

## BlackBoxExporter Package

### Package Location

`github.com/openshift/route-monitor-operator/pkg/blackboxexporter`

### Main Components

#### `BlackBoxExporter` Struct

```go
type BlackBoxExporter struct {
    Client         client.Client              // API client
    Log            logr.Logger                // Logger instance
    Ctx            context.Context            // Context for operations
    Image          string                     // Blackbox Exporter container image URI
    NamespacedName types.NamespacedName       // NamespacedName (name + namespace)
}
```

### Key Methods

#### Initialization

**`New()`**
```go
func New(
    client client.Client,
    log logr.Logger,
    ctx context.Context,
    blackBoxImage string,
    blackBoxExporterNamespace string,
) *BlackBoxExporter
```
- Creates and initializes a new BlackBoxExporter instance
- Parameters:
  - `client`: API client for resource management
  - `log`: Logger for operational logging
  - `ctx`: Context for API operations
  - `blackBoxImage`: Container image URI (e.g., `quay.io/prometheus/blackbox-exporter:v0.23.0`)
  - `blackBoxExporterNamespace`: namespace for deployment
- Returns: Configured `*BlackBoxExporter` instance

#### Namespace Operations

**`GetBlackBoxExporterNamespace()`**
- Returns the namespace where Blackbox Exporter is deployed
- Useful for resource lookups and logging

#### Resource Lifecycle Methods

**Creation Methods:**

1. **`EnsureBlackBoxExporterDeploymentExists()`**
   - Ensures Blackbox Exporter Deployment exists in cluster
   - Creates if missing, updates if different from template
   - Deployment specifications:
     - 1 replica (hardcoded)
     - Image: User-provided container image
     - Port: 9115 (Blackbox Exporter standard)
     - Volume: ConfigMap mount at `/config`
     - Node affinity: Prefers infra/master nodes
     - Tolerations: Tolerates node taints
   - Handles cluster version compatibility (4.13+)
   - Detects private vs public NLB configuration

2. **`EnsureBlackBoxExporterServiceExists()`**
   - Ensures Service exists for metrics exposure
   - Creates if missing
   - Service specifications:
     - Type: ClusterIP
     - Port: 9115
     - Port name: "blackbox"
     - Selector: Matches Blackbox Exporter pod labels

3. **`EnsureBlackBoxExporterConfigMapExists()`**
   - Ensures ConfigMap exists with probe module configurations
   - Creates if missing
   - Contains `blackbox.yaml` with probe modules:
     - `http_2xx`: Standard HTTP probing
     - `insecure_http_2xx`: HTTP with TLS verification disabled

4. **`EnsureBlackBoxExporterResourcesExist()`**
   - Orchestrates creation of all resources in dependency order:
     1. ConfigMap (needed for Deployment volume)
     2. Deployment (Blackbox Exporter pod)
     3. Service (exposes metrics after Deployment ready)

**Deletion Methods:**

1. **`EnsureBlackBoxExporterDeploymentAbsent()`**
   - Deletes Blackbox Exporter Deployment if exists
   - Gracefully handles already-deleted resources

2. **`EnsureBlackBoxExporterServiceAbsent()`**
   - Deletes Blackbox Exporter Service if exists

3. **`EnsureBlackBoxExporterConfigMapAbsent()`**
   - Deletes Blackbox Exporter ConfigMap if exists

4. **`EnsureBlackBoxExporterResourcesAbsent()`**
   - Orchestrates deletion of all resources in order:
     1. Service
     2. Deployment
     3. ConfigMap

#### Dependency Management

**`ShouldDeleteBlackBoxExporterResources()`**
```go
func (b *BlackBoxExporter) ShouldDeleteBlackBoxExporterResources() (
    blackboxexporter.ShouldDeleteBlackBoxExporter, 
    error,
)
```
- Determines if Blackbox Exporter resources should be deleted
- Logic:
  - Counts RouteMonitor resources in cluster
  - Counts ClusterUrlMonitor resources in cluster
  - Returns `DeleteBlackBoxExporter` (true) if:
    - Exactly 1 dependent resource exists AND
    - That resource is marked for deletion (has DeletionTimestamp)
  - Returns `KeepBlackBoxExporter` (false) otherwise
- Prevents premature deletion when multiple monitors exist
- Enables clean removal when last monitor is deleted

### Constants and Types

Located in `pkg/consts/blackboxexporter/`:

```go
const (
    BlackBoxExporterName       = "blackbox-exporter"
    BlackBoxExporterPortName   = "blackbox"
    BlackBoxExporterPortNumber = 9115
)

type ShouldDeleteBlackBoxExporter bool

var (
    KeepBlackBoxExporter   ShouldDeleteBlackBoxExporter = false
    DeleteBlackBoxExporter ShouldDeleteBlackBoxExporter = true
)

// Helper function for labeling resources
func GenerateBlackBoxExporterLabels() map[string]string {
    return map[string]string{"app": BlackBoxExporterName}
}
```

---

## Deployment and Configuration

### Resources Created

#### 1. Deployment Resource

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: blackbox-exporter
  namespace: openshift-monitoring  # or custom namespace
  labels:
    app: blackbox-exporter
spec:
  replicas: 1
  selector:
    matchLabels:
      app: blackbox-exporter
  template:
    metadata:
      labels:
        app: blackbox-exporter
    spec:
      affinity:
        nodeAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - preference:
              matchExpressions:
              - key: node-role.kubernetes.io/infra
                operator: Exists
            weight: 1
      tolerations:
      - operator: Exists
        effect: NoSchedule
        key: node-role.kubernetes.io/infra
      containers:
      - name: blackbox-exporter
        image: quay.io/prometheus/blackbox-exporter:latest
        args:
        - --config.file=/config/blackbox.yaml
        ports:
        - containerPort: 9115
          name: blackbox
        volumeMounts:
        - name: blackbox-config
          readOnly: true
          mountPath: /config
      volumes:
      - name: blackbox-config
        configMap:
          name: blackbox-exporter
```

#### 2. Service Resource

```yaml
apiVersion: v1
kind: Service
metadata:
  name: blackbox-exporter
  namespace: openshift-monitoring
  labels:
    app: blackbox-exporter
spec:
  type: ClusterIP
  selector:
    app: blackbox-exporter
  ports:
  - name: blackbox
    port: 9115
    targetPort: blackbox
```

#### 3. ConfigMap Resource

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: blackbox-exporter
  namespace: openshift-monitoring
  labels:
    app: blackbox-exporter
data:
  blackbox.yaml: |
    modules:
      http_2xx:
        prober: http
        timeout: 15s
      insecure_http_2xx:
        prober: http
        timeout: 15s
        http:
          tls_config:
            insecure_skip_verify: true
```

### Image Configuration

The container image is specified when initializing BlackBoxExporter:

```go
bbe := blackboxexporter.New(
    client,
    logger,
    ctx,
    "quay.io/prometheus/blackbox-exporter:v0.23.0",  // Image URI
    "openshift-monitoring",                             // Namespace
)
```

**Recommended Images:**
- `quay.io/prometheus/blackbox-exporter:latest` - Latest version
- `quay.io/prometheus/blackbox-exporter:v0.23.0` - Specific version for stability
- Custom registry versions for air-gapped environments

### Probe Timeout Configuration

Current configuration uses 15-second timeout for HTTP probes:

```yaml
http_2xx:
  prober: http
  timeout: 15s
```

This means:
- Each probe waits maximum 15 seconds for response
- If endpoint doesn't respond in 15 seconds, probe is marked as failed
- Adequate for most route monitoring scenarios

### Node Affinity Strategy

The package automatically selects node affinity based on:

**Clusters with versions < 4.13:**
- Prefers: `node-role.kubernetes.io/infra`

**Clusters with versions >= 4.13 with Private NLB:**
- Prefers: `node-role.kubernetes.io/master`

**Tolerance Configuration:**
- Pod tolerates `NoSchedule` taint on preferred node role
- Allows scheduling on tainted nodes if needed

---

## Troubleshooting and Monitoring

### Common Issues and Solutions

#### Issue 1: Blackbox Exporter Pod Not Starting

**Symptoms:**
- Pod stuck in `Pending` or `ImagePullBackOff` state
- Deployment shows 0/1 ready replicas

**Diagnostic Steps:**
```bash
# Check pod status
oc get pods -n openshift-monitoring | grep blackbox-exporter

# View pod events and logs
oc describe pod <pod-name> -n openshift-monitoring
oc logs <pod-name> -n openshift-monitoring

# Check node affinity constraints
oc get nodes -L node-role.kubernetes.io/infra,node-role.kubernetes.io/master
```

**Solutions:**
1. **Image Pull Error**: Verify image URI is correct and accessible
   ```bash
   oc get deployment blackbox-exporter -n openshift-monitoring -o yaml | grep image
   ```

2. **Node Not Found**: If no infra nodes exist, check node labels
   ```bash
   oc get nodes --show-labels | grep infra
   oc get nodes --show-labels | grep master
   ```

3. **Node Taint Issue**: Verify toleration matches taint
   ```bash
   oc describe nodes <node-name> | grep Taints
   ```

#### Issue 2: Probe Failures - Service Cannot Reach Blackbox Exporter

**Symptoms:**
- Service shows no endpoints
- Pods show errors connecting to Blackbox Exporter

**Diagnostic Steps:**
```bash
# Check Service endpoints
oc get endpoints blackbox-exporter -n openshift-monitoring

# Verify pod labels match Service selector
oc get pod <pod-name> -n openshift-monitoring -o yaml | grep labels

# Test connectivity from another pod
oc run -it --rm debug --image=busybox -- sh
wget http://blackbox-exporter:9115/metrics
```

**Solutions:**
1. **Label Mismatch**: Ensure pod labels match Service selector
   ```bash
   oc get svc blackbox-exporter -n openshift-monitoring -o yaml | grep selector
   ```

2. **Pod Not Running**: Wait for Deployment rollout
   ```bash
   oc rollout status deployment/blackbox-exporter -n openshift-monitoring
   ```

#### Issue 3: Routes Not Being Probed

**Symptoms:**
- Probe metrics show all failures
- No HTTP responses from route endpoints

**Diagnostic Steps:**
```bash
# Check probe results directly
oc port-forward svc/blackbox-exporter 9115:9115 -n openshift-monitoring
curl 'http://localhost:9115/probe?target=https://route-url&module=http_2xx'

# Check route accessibility
oc run -it --rm debug --image=curlimages/curl -- sh
curl -v https://route-url

# Verify probe module in ConfigMap
oc get cm blackbox-exporter -n openshift-monitoring -o yaml
```

**Solutions:**
1. **Route Not Accessible**: Verify route is externally accessible
   - Check route definition
   - Verify DNS resolution
   - Check service backend health

2. **TLS Certificate Issue**: Use `insecure_http_2xx` module for self-signed certs
   ```yaml
   modules:
     insecure_http_2xx:
       prober: http
       timeout: 15s
       http:
         tls_config:
           insecure_skip_verify: true
   ```

3. **Unexpected Status Code**: Update expected status codes in ConfigMap
   ```yaml
   http:
     valid_status_codes:
       - 200
       - 201
       - 202
   ```

#### Issue 4: ConfigMap Not Mounted Correctly

**Symptoms:**
- Pod fails with "config file not found" error
- Blackbox Exporter can't read probe configurations

**Diagnostic Steps:**
```bash
# Check pod volume mounts
oc describe pod <pod-name> -n openshift-monitoring | grep -A 5 Mounts

# Verify ConfigMap exists
oc get cm -n openshift-monitoring | grep blackbox-exporter

# Check pod startup logs
oc logs <pod-name> -n openshift-monitoring --tail=50
```

**Solutions:**
1. **ConfigMap Missing**: Verify `EnsureBlackBoxExporterConfigMapExists()` completed
   ```bash
   oc get cm blackbox-exporter -n openshift-monitoring
   ```

2. **Mount Path Issue**: Check volume mount path is `/config`
   ```bash
   oc exec <pod-name> -n openshift-monitoring -- ls -la /config/
   ```

### Monitoring Blackbox Exporter Health

#### Key Metrics to Watch

```promql
# Probe success rate
rate(probe_success[5m])

# Average probe duration
rate(probe_duration_seconds_sum[5m]) / rate(probe_duration_seconds_count[5m])

# Failed probes in last 5 minutes
increase(probe_success{probe_success="0"}[5m])

# Certificate expiry warning (< 7 days)
probe_ssl_earliest_cert_expiry - time() < 7 * 24 * 3600
```

### Viewing Metrics

#### From within cluster:
```bash
# Port forward to Blackbox Exporter service
oc port-forward svc/blackbox-exporter 9115:9115 -n openshift-monitoring

# Access metrics endpoint
curl http://localhost:9115/metrics

# Test specific probe
curl 'http://localhost:9115/probe?target=https://example.com&module=http_2xx'
```

#### From Prometheus UI:
1. Access Prometheus dashboard
2. Go to Graph tab
3. Query `probe_success` to see all probes
4. Filter by labels: `{job="blackbox-exporter"}`
