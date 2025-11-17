# Route Monitor Operator - E2E Tests

## Cluster-Based E2E Tests

Tests against a real OpenShift cluster.

### Running

1. Build the test binary: `make e2e-binary-build`
2. Deploy your operator in a test cluster
3. Install Ginkgo: `go install github.com/onsi/ginkgo/ginkgo@latest`
4. Get kubeconfig: `ocm get /api/clusters_mgmt/v1/clusters/(cluster-id)/credentials | jq -r .kubeconfig > kubeconfig`
5. Run tests: `DISABLE_JUNIT_REPORT=true KUBECONFIG=./kubeconfig ./bin/ginkgo --tags=osde2e -v test/e2e`

---

## Full Integration Tests

Tests the complete RMO → RHOBS API → Agent integration locally without a cluster.

### What It Tests

```
RMO (fake K8s) → API (local storage) → Agent (fetches probes)
```

- RMO creates probe from HostedControlPlane CR
- API stores the probe configuration
- Agent fetches the probe
- Test mocks agent processing (agent needs real K8s to deploy resources)

### Setup

Clone the RHOBS repos as sibling directories:

```bash
cd ..
git clone https://github.com/rhobs/rhobs-synthetics-api.git
git clone https://github.com/rhobs/rhobs-synthetics-agent.git
cd route-monitor-operator
```

Or set custom paths:

```bash
export RHOBS_SYNTHETICS_API_PATH=/path/to/rhobs-synthetics-api
export RHOBS_SYNTHETICS_AGENT_PATH=/path/to/rhobs-synthetics-agent
```

### Running

```bash
make test-e2e-full
```

The test builds API/Agent binaries from source and runs everything locally (~20 seconds).

### Note on Agent Processing

The agent fetches probes but cannot fully process them without a real Kubernetes cluster (needs to deploy Prometheus/blackbox-exporter). The test mocks this by updating the probe status directly via the API.
