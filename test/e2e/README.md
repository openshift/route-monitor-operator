# Route Monitor Operator - E2E Test Suite

This directory contains end-to-end tests for the Route Monitor Operator (RMO), validating its integration with the RHOBS synthetic monitoring system.

## Table of Contents

- [Overview](#overview)
- [Test Files](#test-files)
- [Local E2E Testing](#local-e2e-testing)
- [CI/CD Testing (osde2e)](#cicd-testing-osde2e)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)

---

## Overview

The RMO e2e test suite validates the complete synthetic monitoring workflow:

```
HostedControlPlane CR → RMO → RHOBS API → Synthetics Agent → Prometheus/Blackbox Exporter
```

**What We Test:**
- RMO watches HostedControlPlane CRs and creates synthetic probes
- Public vs private cluster detection (via VpcEndpoint CRs)
- Probe lifecycle: creation, updates, deletion
- Retry logic and error handling (SREP-2832, SREP-2966)
- Integration with RHOBS Synthetics API

**What We Don't Test:**
- Real HCP cluster provisioning (too slow/expensive for e2e tests)
- Actual probe execution and metrics collection (tested separately in rhobs-synthetics-agent)
- Network connectivity to real API servers (we use fake endpoints)

---

## Test Files

### `route_monitor_operator_tests.go` (osde2e)

**Build Tag:** `//go:build osde2e`

**Test Suites:**

#### Suite 1: "Route Monitor Operator" - Basic Installation
Tests RMO deployment and basic functionality:

| Test | Description |
|------|-------------|
| `is installed` | Verifies RMO deployment exists and is ready |
| `can be upgraded` | Pending - upgrade testing |
| `has all of the required resources` | Checks for Deployment, ServiceAccount, ClusterRole, ClusterRoleBinding |
| `required dependent resources are created` | Verifies RouteMonitor CRs are created for test routes |

#### Suite 2: "RHOBS Synthetic Monitoring" - HostedControlPlane Integration
Tests synthetic monitoring workflow with simulated Management Cluster environment:

| Test | Description | What It Validates |
|------|-------------|-------------------|
| `has RHOBS monitoring configured` | Checks RHOBS credentials in ConfigMap | OIDC auth setup |
| `creates probe for public HostedControlPlane` | Creates fake public HCP CR, waits for probe creation | Public cluster detection, probe creation with `private=false` label |
| `creates probe for private HostedControlPlane` | Creates fake private HCP with VpcEndpoint CR | Private cluster detection via VpcEndpoint, probe with `private=true` label |
| `deletes probe when HostedControlPlane is deleted` | Deletes HCP, verifies probe cleanup | Finalizer logic, probe deletion, no orphaned probes |

**Key Feature:** These tests simulate a Management Cluster by creating fake HostedControlPlane and VpcEndpoint CRs that match production patterns exactly.

**Environment Detection:** Tests detect whether they're running in integration vs staging by querying the osde2e cluster provider (`provider.Environment()`). However, the RHOBS API endpoint URL is **not** auto-detected - it must be explicitly configured via the `PROBE_API_URL` environment variable, which is automatically injected by app-interface based on the target environment.

### `full_integration_test.go` (e2e)

**Build Tag:** `//go:build e2e`

**Test:** `TestFullStackIntegration`

**Purpose:** Validates the complete RMO → API → Agent flow locally without a real cluster.

**What It Tests:**
1. RMO creates a probe from a fake HostedControlPlane CR
2. RHOBS Synthetics API stores the probe configuration
3. RHOBS Synthetics Agent fetches the probe from API
4. Test mocks agent processing (agent needs real K8s to deploy resources)

**Requirements:**
- Local clones of `rhobs-synthetics-api` and `rhobs-synthetics-agent` repos
- No Kubernetes cluster or Docker needed
- Runs in ~20 seconds

**What Gets Tested:**
- RMO controller logic with fake K8s client
- API CRUD operations (file-based storage)
- Agent API polling
- End-to-end data flow

**What Gets Mocked:**
- Kubernetes cluster (using `fake.NewClientBuilder()`)
- Agent resource deployment (test updates probe status directly)
- Dynatrace endpoints
- Probe target endpoints

### `probe_deletion_retry_test.go` (e2e)

**Build Tag:** `//go:build e2e`

**Test:** `TestProbeDeletionRetry`

**Purpose:** Validates SREP-2832 + SREP-2966 fixes (hybrid retry-then-fail-open approach).

**Scenario:**
1. Create probe normally via API
2. Stop API to simulate unavailability
3. Attempt probe deletion via RMO
4. Verify RMO returns error (fail closed) - prevents orphaned probes
5. Verify finalizer is NOT removed (blocks HCP deletion)
6. Restart API
7. Verify probe deletion succeeds on retry

**What It Validates:**
- **Within 15-minute timeout:** RMO fails closed (retries, blocks deletion)
- **After 15-minute timeout:** RMO fails open (allows deletion to prevent indefinite blocking)
- Structured logging with behavior indicators (`fail_closed`, `fail_open`)
- No orphaned probes during transient API failures
- HCP deletion not blocked indefinitely during prolonged API outages

**Related Issues:**
- SREP-2832: Orphaned probes not being garbage collected
- SREP-2966: RMO blocking cluster deletion indefinitely

### `helpers.go` (e2e)

**Build Tag:** `//go:build e2e`

**Purpose:** Shared utilities for local e2e tests.

**Key Functions:**

| Function | Description |
|----------|-------------|
| `createProbeViaAPI()` | Creates a probe directly through RHOBS API |
| `getProbeByID()` | Fetches probe by ID from API |
| `deleteProbeByID()` | Deletes probe from API |
| `updateProbeStatus()` | Mocks agent behavior by updating probe status |
| `startMockDynatraceServer()` | HTTP server mocking Dynatrace endpoints |
| `startMockProbeTargetServer()` | HTTP server mocking probe target endpoints |
| `testWriter` | Captures logs for validation in tests |

**Why We Mock Agent Behavior:**
The real agent needs a Kubernetes cluster to deploy Prometheus and Blackbox Exporter resources. In local tests, we bypass this by updating probe status directly via the API.

### `api_manager.go` (e2e)

**Build Tag:** `//go:build e2e`

**Purpose:** Manages RHOBS Synthetics API lifecycle in tests.

**Key Functions:**
- `NewRealAPIManager()` - Creates manager with auto-detected or custom API path
- `Start()` - Builds API binary and starts server
- `Stop()` - Gracefully stops API server
- `GetURL()` - Returns API base URL
- `ClearAllProbes()` - Cleanup utility for test isolation

### `agent_manager.go` (e2e)

**Build Tag:** `//go:build e2e`

**Purpose:** Manages RHOBS Synthetics Agent lifecycle in tests.

**Key Functions:**
- `NewAgentManager()` - Creates manager with auto-detected or custom agent path
- `Start()` - Builds agent binary and starts it
- `Stop()` - Gracefully stops agent process

### `process_manager.go` (e2e)

**Build Tag:** `//go:build e2e`

**Purpose:** Low-level process management utilities for starting/stopping Go binaries in tests.

### `route_monitor_operator_runner_test.go` (osde2e)

**Build Tag:** `//go:build osde2e`

**Purpose:** Test runner that initializes Ginkgo test suite for osde2e tests.

**Key Function:**
```go
func TestRouteMonitorOperator(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "Route Monitor Operator Suite")
}
```

### `run-e2e.sh`

**Purpose:** Bash wrapper for running local e2e tests.

**What It Does:**
1. Auto-detects or validates RHOBS repo paths
2. Exports environment variables
3. Runs `TestFullStackIntegration` with 5-minute timeout
4. Reports success/failure

**Usage:**
```bash
cd test/e2e
./run-e2e.sh
```

---

## Local E2E Testing

These tests run completely locally on your development machine without requiring a Kubernetes cluster or CI/CD infrastructure.

**Build Tag:** `//go:build e2e`

**Files:** `full_integration_test.go`, `probe_deletion_retry_test.go`, `helpers.go`, `api_manager.go`, `agent_manager.go`, `process_manager.go`, `run-e2e.sh`

### What Gets Tested Locally

- **Full Stack Integration:** RMO → RHOBS API → Synthetics Agent flow
- **Probe Deletion Retry Logic:** SREP-2832 + SREP-2966 fixes (hybrid retry-then-fail-open)
- **Controller Logic:** Using fake Kubernetes clients
- **API Operations:** CRUD operations with file-based storage
- **Agent Behavior:** API polling and probe fetching

### What Gets Mocked

- Kubernetes cluster (using `fake.NewClientBuilder()`)
- Agent resource deployment (test updates probe status directly via API)
- Dynatrace endpoints
- Probe target endpoints

### Prerequisites

Clone RHOBS repos as sibling directories:

```bash
cd /path/to/repos
git clone https://github.com/openshift/route-monitor-operator.git
git clone https://github.com/rhobs/rhobs-synthetics-api.git
git clone https://github.com/rhobs/rhobs-synthetics-agent.git
```

**OR** set custom paths:

```bash
export RHOBS_SYNTHETICS_API_PATH=/path/to/rhobs-synthetics-api
export RHOBS_SYNTHETICS_AGENT_PATH=/path/to/rhobs-synthetics-agent
```

### Running Local Tests

**Full integration test (RMO → API → Agent):**
```bash
# From repo root
make test-e2e-full

# Or directly
cd test/e2e
./run-e2e.sh
```

**Probe deletion retry test:**
```bash
cd test/e2e
go test -v -tags=e2e -timeout=5m . -run TestProbeDeletionRetry
```

**Run all local e2e tests:**
```bash
cd test/e2e
go test -v -tags=e2e -timeout=10m .
```

### Environment Variables (Local)

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `RHOBS_SYNTHETICS_API_PATH` | Yes* | `../rhobs-synthetics-api` | Path to API repo |
| `RHOBS_SYNTHETICS_AGENT_PATH` | Yes* | `../rhobs-synthetics-agent` | Path to Agent repo |

*Auto-detected if repos are sibling directories

### Local Test Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Test Process                                                 │
│                                                              │
│  ┌──────────────┐                                           │
│  │ RMO          │ (fake K8s client)                         │
│  │ Controller   │──┐                                        │
│  └──────────────┘  │                                        │
│                    │ HTTP POST /probes                      │
│                    ▼                                         │
│  ┌──────────────────────────────┐                           │
│  │ RHOBS Synthetics API         │ (local Go process)        │
│  │ (file-based storage)         │                           │
│  └──────────────────────────────┘                           │
│                    │                                         │
│                    │ HTTP GET /probes                        │
│                    ▼                                         │
│  ┌──────────────────────────────┐                           │
│  │ RHOBS Synthetics Agent       │ (local Go process)        │
│  │ (fetches probes)             │                           │
│  └──────────────────────────────┘                           │
│                                                              │
│  Mock Servers:                                               │
│  - Dynatrace endpoint (HTTP 200)                            │
│  - Probe targets (/livez, /readyz endpoints)                │
└─────────────────────────────────────────────────────────────┘
```

---

## CI/CD Testing (osde2e)

These tests run automatically in the CI/CD pipeline on real OpenShift clusters managed by the osde2e framework.

**Build Tag:** `//go:build osde2e`

**Files:** `route_monitor_operator_tests.go`, `route_monitor_operator_runner_test.go`, `e2e-template.yml`

### What Gets Tested in CI/CD

**Suite 1: "Route Monitor Operator" - Basic Installation**
- RMO deployment exists and is ready
- All required resources present (Deployment, ServiceAccount, ClusterRole, etc.)
- RouteMonitor CRs created for test routes

**Suite 2: "RHOBS Synthetic Monitoring" - HostedControlPlane Integration**
- RHOBS credentials configured correctly
- Probe creation for **public** HostedControlPlane (with `private=false` label)
- Probe creation for **private** HostedControlPlane (with `private=true` label, VpcEndpoint detection)
- Probe deletion and finalizer cleanup when HCP is deleted

### How CI/CD Tests Work

**Key Feature:** Tests simulate a Management Cluster by creating **fake HostedControlPlane and VpcEndpoint CRs** that match production patterns exactly. No actual HCP clusters are provisioned.

### CI/CD Pipeline Flow

**Configuration:** [`osde2e-focus-test.yaml`](https://gitlab.cee.redhat.com/service/app-interface/-/blob/master/data/services/osd-operators/cicd/saas/saas-route-monitor-operator/osde2e-focus-test.yaml)

1. **Cluster Provisioning:** osde2e framework creates or uses existing test cluster (`USE_EXISTING_CLUSTER=TRUE`)

2. **Secret Injection:** Environment variables **automatically loaded from Vault**:
   - `EXTERNAL_SECRET_OIDC_CLIENT_ID`
   - `EXTERNAL_SECRET_OIDC_CLIENT_SECRET`
   - `EXTERNAL_SECRET_OIDC_ISSUER_URL`
   - `PROBE_API_URL`
   - `SKIP_INFRASTRUCTURE_HEALTH_CHECK`

3. **CRD Installation:** Test installs required CRDs (HostedControlPlane, VpcEndpoint) in `BeforeAll`

4. **Test Execution:** Ginkgo runs tests tagged with `//go:build osde2e`

5. **Environment Detection:** Tests detect environment name (integration/staging) via osde2e provider for logging purposes. RHOBS API endpoint is explicitly configured via `PROBE_API_URL` environment variable

### CI/CD Environments

| Environment | Management Cluster | RHOBS API Endpoint |
|-------------|-------------------|-------------------|
| **Integration** | `hivei01ue1` (us-east-1) | `https://us-west-2.rhobs.api.integration.openshift.com/api/metrics/v1/hcp/probes` |
| **Staging** | `hives02ue1` (us-east-1) | `https://us-east-1-0.rhobs.api.stage.openshift.com/api/metrics/v1/hcp/probes` |

### Vault Secret Paths

Secrets are stored in Vault and automatically injected by osde2e:

- **Integration:** `osd-sre/integration/route-monitor-operator-credentials`
- **Staging:** `osd-sre/staging/route-monitor-operator-credentials`

**Required Fields:**
- `OIDC_CLIENT_ID`
- `OIDC_CLIENT_SECRET`
- `OIDC_ISSUER_URL`

### Pipeline Triggers

- **Automatic:** Every merge to main branch
- **Promotion-based:** Integration tests must pass before staging deployment
- **Manual:** Via app-interface MR

**Test Image:** `quay.io/redhat-services-prod/openshift/route-monitor-operator-e2e`

### Running osde2e Tests Manually

**Prerequisites:**
1. Access to a real OpenShift cluster (integration or staging)
2. RMO deployed on the cluster
3. RHOBS credentials configured

**Option 1: Using Ginkgo**
```bash
# Install ginkgo
go install github.com/onsi/ginkgo/v2/ginkgo@latest

# Get cluster kubeconfig
ocm get /api/clusters_mgmt/v1/clusters/<cluster-id>/credentials | jq -r .kubeconfig > /tmp/kubeconfig

# Run all tests
DISABLE_JUNIT_REPORT=true \
KUBECONFIG=/tmp/kubeconfig \
ginkgo --tags=osde2e -v test/e2e
```

**Option 2: Running specific test suites**
```bash
# Only basic installation tests
ginkgo --tags=osde2e -v --focus="Route Monitor Operator" test/e2e

# Only RHOBS synthetic monitoring tests
ginkgo --tags=osde2e -v --focus="RHOBS Synthetic Monitoring" test/e2e
```

### Environment Variables (CI/CD)

| Variable | Required | Source | Description |
|----------|----------|--------|-------------|
| `EXTERNAL_SECRET_OIDC_CLIENT_ID` | Yes | Vault (auto-loaded) | OIDC client ID for RHOBS API auth |
| `EXTERNAL_SECRET_OIDC_CLIENT_SECRET` | Yes | Vault (auto-loaded) | OIDC client secret |
| `EXTERNAL_SECRET_OIDC_ISSUER_URL` | Yes | Vault (auto-loaded) | OIDC issuer URL |
| `PROBE_API_URL` | Yes | app-interface config | RHOBS API endpoint URL |
| `SKIP_INFRASTRUCTURE_HEALTH_CHECK` | No | `"true"` | Skip infra checks for test HCPs |
| `KUBECONFIG` | Yes | Manual/OCM | Path to cluster credentials |

### CI/CD Test Architecture

```
┌─────────────────────────────────────────────────────────────┐
│ Real OpenShift Cluster (Integration/Staging)                │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ Test Namespace                                        │  │
│  │                                                       │  │
│  │  ┌─────────────────────────────────────────────┐    │  │
│  │  │ Fake HostedControlPlane CR                  │    │  │
│  │  │ - Matches production MC patterns            │    │  │
│  │  │ - Status manually set to Available=True     │    │  │
│  │  └─────────────────────────────────────────────┘    │  │
│  │                                                       │  │
│  │  ┌─────────────────────────────────────────────┐    │  │
│  │  │ Fake VpcEndpoint CR (for private tests)     │    │  │
│  │  │ - purpose: backplane label                  │    │  │
│  │  │ - Fake VPC endpoint ID                      │    │  │
│  │  └─────────────────────────────────────────────┘    │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ openshift-route-monitor-operator namespace           │  │
│  │                                                       │  │
│  │  RMO watches fake HCPs and calls real RHOBS API     │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                              │
└─────────────────────────────────────────────────────────────┘
                              │
                              │ HTTPS (OIDC auth)
                              ▼
┌─────────────────────────────────────────────────────────────┐
│ RHOBS Cell (rhobsi01uw2 or rhobs staging)                  │
│                                                              │
│  Real RHOBS Synthetics API creates actual probe objects    │
└─────────────────────────────────────────────────────────────┘
```

**Key Insight:** osde2e tests use **fake CRs that look real** to test RMO logic without provisioning actual HCP clusters. Fast (30-40s per test), cost-effective, validates real controller behavior.

---

## Troubleshooting

### Local E2E Test Issues

#### `RHOBS Synthetics API path not set`

The test cannot find the RHOBS API or Agent repositories.

**Solution:**
```bash
# Option 1: Clone as sibling directories
cd /path/to/repos
git clone https://github.com/rhobs/rhobs-synthetics-api.git
git clone https://github.com/rhobs/rhobs-synthetics-agent.git

# Option 2: Set environment variables
export RHOBS_SYNTHETICS_API_PATH=/custom/path/to/api
export RHOBS_SYNTHETICS_AGENT_PATH=/custom/path/to/agent
```

#### `Build failed` or `Binary not found`

The test cannot compile the API or Agent binaries.

**Solution:** Ensure RHOBS repos are up-to-date and have no build errors:
```bash
cd $RHOBS_SYNTHETICS_API_PATH && go build ./cmd/rhobs-synthetics-api
cd $RHOBS_SYNTHETICS_AGENT_PATH && go build ./cmd/rhobs-synthetics-agent
```

#### Test hangs or times out

The test gets stuck waiting for something.

**Solution:**
- Check if ports 8080 (API) or other random ports are already in use
- Verify mock servers started successfully (check test logs)
- Run with increased timeout: `go test -timeout=10m ...`

---

### CI/CD (osde2e) Test Issues

#### `no OIDC credentials found`

The test cannot authenticate with RHOBS API.

**Solution:** Verify secrets exist in Vault and are properly configured in app-interface:
```bash
# Check integration credentials
vault kv get osd-sre/integration/route-monitor-operator-credentials

# Check staging credentials
vault kv get osd-sre/staging/route-monitor-operator-credentials
```

Ensure these secrets have the required fields:
- `OIDC_CLIENT_ID`
- `OIDC_CLIENT_SECRET`
- `OIDC_ISSUER_URL`

#### `no matches for kind "HostedControlPlane"`

Required CRDs are not installed on the test cluster.

**Solution:** The test should install CRDs automatically in `BeforeAll`. If this fails:
- Check test logs for CRD installation errors
- Verify cluster has permissions to create CRDs
- The CRD definitions are embedded in `route_monitor_operator_tests.go` (lines 46-96)

#### Tests can't reach RHOBS API

Network connectivity or authentication issues with RHOBS endpoints.

**Solution:**
- Verify `PROBE_API_URL` is correct for your environment:
  - Integration: `https://us-west-2.rhobs.api.integration.openshift.com/api/metrics/v1/hcp/probes`
  - Staging: `https://us-east-1-0.rhobs.api.stage.openshift.com/api/metrics/v1/hcp/probes`
- Check OIDC credentials are valid
- Test authentication manually from the test cluster
- Verify firewall/network policies allow outbound HTTPS to RHOBS endpoints

#### Test creates orphaned probes

Probes remain in RHOBS after test completes.

**Solution:** Tests should clean up automatically using finalizers. For manual cleanup:
```bash
# List all probes with test cluster-id pattern
curl -H "Authorization: Bearer <token>" \
  https://<rhobs-api>/api/metrics/v1/hcp/probes?cluster-id=test-osde2e-*

# Delete specific probe
curl -X DELETE -H "Authorization: Bearer <token>" \
  https://<rhobs-api>/api/metrics/v1/hcp/probes/<probe-id>
```

#### PROBE_API_URL not set or incorrect

Test fails because it cannot connect to RHOBS API or connects to wrong environment.

**Explanation:** The PROBE_API_URL is NOT auto-detected. It must be explicitly set via environment variable.

**Solution:**

In CI/CD, this is automatically set by app-interface configuration for each environment. For manual testing, set it explicitly:

```bash
# For integration environment
export PROBE_API_URL="https://us-west-2.rhobs.api.integration.openshift.com/api/metrics/v1/hcp/probes"

# For staging environment
export PROBE_API_URL="https://us-east-1-0.rhobs.api.stage.openshift.com/api/metrics/v1/hcp/probes"

# Then run tests
ginkgo --tags=osde2e -v test/e2e
```

---

## Related Documentation

- [Full Integration Tests](https://github.com/openshift/route-monitor-operator/blob/main/test/e2e/FULL_INTEGRATION_TESTS.md) - Deep dive on local e2e tests
- [app-interface osde2e config](https://gitlab.cee.redhat.com/service/app-interface/-/blob/master/data/services/osd-operators/cicd/saas/saas-route-monitor-operator/osde2e-focus-test.yaml) - CI/CD configuration
- [Route Monitor Operator GitHub](https://github.com/openshift/route-monitor-operator) - Main repository

---

## Contributing

When adding new tests:

1. **Choose the right test type:**
   - osde2e (`//go:build osde2e`) - For integration tests requiring real K8s
   - e2e (`//go:build e2e`) - For fast, local unit/integration tests

2. **Add build tags:** First line of test file must be `//go:build <tag>`

3. **Use helpers:** Reuse functions from `helpers.go` instead of duplicating

4. **Clean up resources:** Use `defer` or `AfterEach` to clean up test resources

5. **Update this documentation:** Document new tests in the [Test Files](#test-files) section

6. **Test locally first:** Run tests locally before pushing:
   ```bash
   make test-e2e-full  # Local e2e tests
   ```

---

**Last Updated:** 2026-02-12
**Maintainers:** ROSA Rocket Team (@team-rosa-rocket)
**Slack:** [#team-rosa-rocket](https://redhat-internal.slack.com/archives/C08N5S632V8)
