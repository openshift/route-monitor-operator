# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

The Route Monitor Operator is a Kubernetes operator that automatically enables blackbox probes for OpenShift routes to be consumed by the Cluster Monitoring Operator or Prometheus Operator. It creates and manages ServiceMonitors based on RouteMonitor and ClusterUrlMonitor custom resources.

## Development Commands

### Testing
- `make test` - Run unit tests with coverage
- `make test-integration` - Run integration tests against a cluster (requires login)
- Tests use Ginkgo and Gomega frameworks with uber-go/mock for mocking
- Generate mocks: `make go-generate`

### Building and Running
- `make manager` - Build the manager binary
- `make run` - Run operator locally against configured cluster
- `make run-verbose` - Run with debug logging (zap-log-level=5)
- `make docker-build docker-push` - Build and push container image
- `make deploy` - Deploy operator to cluster (requires IMG variable for custom images)

### Code Quality
- `make fmt` - Format Go code
- `make vet` - Run go vet
- `make generate` - Generate code (CRDs, mocks, etc.)
- Linting uses golangci-lint with gosec enabled (see .golangci.yaml)

### Cluster Operations
- `make install` - Install CRDs to cluster
- `make uninstall` - Remove CRDs from cluster
- Check logs: `oc logs -n openshift-monitoring deploy/route-monitor-operator-controller-manager -c manager`

## Architecture

### Core Components

**Controllers:**
- `controllers/routemonitor/` - Manages RouteMonitor CRs and creates ServiceMonitors for OpenShift routes
- `controllers/clusterurlmonitor/` - Manages ClusterUrlMonitor CRs for cluster-domain-based URLs
- `controllers/hostedcontrolplane/` - Handles HostedControlPlane monitoring for HyperShift

**Package Structure:**
- `api/v1alpha1/` - Custom Resource Definitions (RouteMonitor, ClusterUrlMonitor)
- `pkg/blackboxexporter/` - Manages blackbox exporter deployment in openshift-monitoring
- `pkg/servicemonitor/` - ServiceMonitor creation and management logic
- `pkg/alert/` - Multi-window multi-burn-rate alerting implementation
- `pkg/util/` - Utilities including templates, reconciliation helpers, and test helpers

### Custom Resources

**RouteMonitor** - Namespace-scoped resource that defines monitoring for OpenShift routes:
- Can reference routes from other namespaces
- Creates ServiceMonitors for blackbox probing

**ClusterUrlMonitor** - Namespace-scoped resource for cluster-domain-based URLs:
- URL format: `<prefix><cluster-domain>:<port><suffix>`
- Used for monitoring cluster services not exposed via routes (e.g., API server)

### Key Dependencies
- Built with Kubebuilder v3 and operator-sdk
- Uses controller-runtime for Kubernetes controllers
- Integrates with Prometheus Operator (ServiceMonitors)
- Supports both standard and RHOBS Prometheus operators

### Testing Structure
- Unit tests use Ginkgo/Gomega with MockHelper from `pkg/util/test/helper/`
- Integration tests in `int/` directory
- Tests follow BeforeEach/JustBeforeEach pattern for setup
- MockHelper reduces boilerplate in controller tests

### Alerting
Implements SRE-style multi-window multi-burn-rate alerts that account for new services with insufficient historical data to prevent false positives on newly created monitors.

## Configuration

### Environment Variables
- `PROBE_API_URL` - Optional URL for RHOBS synthetics integration (experimental)

### Packaging
The operator can be packaged for HyperShift using `make package` which creates kubectl-package compatible manifests in the `packaging/` directory.