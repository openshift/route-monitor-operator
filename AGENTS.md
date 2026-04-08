# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

The route-monitor-operator is a Kubernetes operator that enables blackbox probing of OpenShift routes and cluster URLs for Prometheus-based SLO monitoring. It creates ServiceMonitor and PrometheusRule CRs to drive multi-window, multi-burn-rate alerting.

**Go version**: 1.25.5 | **Operator SDK**: v3 (kubebuilder v3) | **FIPS**: enabled

## Build & Development Commands

```bash
make generate          # CRDs, deepcopy, mocks (mockgen), openapi, kustomize manifests
make generate-check    # Verify generated code matches committed files
make test              # Unit/functional tests with coverage (excludes int/)
make lint              # golangci-lint (gosec only) + OLM validation
make container-lint    # Lint inside boilerplate container (matches CI)
make manager           # Build binary
make run               # Run locally against cluster
make run-verbose       # Run locally with debug logging (zap level 5)
make deploy IMG=<img>  # Deploy to cluster via kustomize
make test-integration  # Integration tests (requires live cluster + oc login)
make test-e2e-full     # Full e2e suite
```

Run a single test:
```bash
go test ./controllers/routemonitor/... -run TestSpecificName -v
```

The `make test` target runs `make generate fmt vet` first, then `go test` on all packages except `int/`.

## Architecture

### CRDs (api/v1alpha1/)

- **RouteMonitor** - monitors a specific OpenShift Route by name/namespace, with optional port and path suffix
- **ClusterUrlMonitor** - monitors a constructed URL from prefix + cluster domain + port + suffix; domain resolved from either `infrastructures/cluster` (`domainRef: infra`) or a HostedControlPlane (`domainRef: hcp`)

Both CRDs have SLO specs (target availability percent, must be >90% and <100%) and produce ServiceMonitor + PrometheusRule resources.

### Controllers (controllers/)

Three reconcilers, all depending on handler interfaces defined in `controllers/interfaces.go`:

1. **RouteMonitorReconciler** (`controllers/routemonitor/`) - resolves Route, ensures blackbox exporter, creates ServiceMonitor/PrometheusRule. Supports both CoreOS (`monitoring.coreos.com`) and RHOBS (`monitoring.rhobs`) ServiceMonitor types.
2. **ClusterUrlMonitorReconciler** (`controllers/clusterurlmonitor/`) - constructs URL from cluster domain, same resource creation flow.
3. **HostedControlPlaneReconciler** (`controllers/hostedcontrolplane/`) - optional (enabled when HCP CRD exists or `--enable-hypershift`), manages RHOBS synthetic probes and optional Dynatrace integration, reconciles every 10 minutes.

### Core Packages (pkg/)

| Package | Purpose |
|---------|---------|
| `blackboxexporter/` | Manages blackbox exporter Deployment + Service lifecycle |
| `servicemonitor/` | Templates ServiceMonitor CRs (CoreOS and RHOBS variants) |
| `alert/` | Generates PrometheusRule CRs with multi-window multi-burn-rate SLO alerts |
| `rhobs/` | HTTP client for RHOBS synthetics probe API (OIDC auth) |
| `dynatrace/` | Optional Dynatrace synthetic monitor integration |
| `reconcile/` | Shared reconciliation logic (error status, SLO parsing, finalizers, cluster ID) |
| `util/` | Cluster version checks, URL validation, finalizer helpers |

### Configuration

Runtime config comes from the `route-monitor-operator-config` ConfigMap in `openshift-route-monitor-operator` namespace, with CLI flags as fallback. Key fields: `probe-api-url`, `probe-tenant`, OIDC credentials, `only-public-clusters`, `skip-infrastructure-health-check`.

## Testing Patterns

- **Framework**: Ginkgo (v1 + v2) + Gomega + uber-go/mock
- **Mock generation**: `//go:generate mockgen` directives in source files; generated mocks land in `pkg/util/test/generated/mocks/`
- **Test structure**: top-level `BeforeEach`/`JustBeforeEach` set defaults, per-`It` blocks override specific fields
- **MockHelper** (`pkg/util/test/helper/helper.go`): simplifies mock setup and call tracking
- **Integration tests** (`int/`): require a live cluster; run via `hack/test-integration.sh`
- **E2E tests** (`test/e2e/`): require `ginkgo` binary and deployed operator; use `--tags=osde2e`

## Build System

The Makefile includes `boilerplate/generated-includes.mk` from the [openshift/boilerplate](https://github.com/openshift/boilerplate) framework, which provides standard targets (`go-build`, `go-test`, `go-generate`, `lint`, `validate`, `docker-build`, `docker-push`, etc.). The project subscribes to `openshift/golang-osd-operator` and `openshift/golang-osd-e2e` conventions.

`make boilerplate-update` pulls the latest boilerplate conventions.

## CI

- **Tekton** (`.tekton/`): PR and push pipelines for builds, e2e tests, and PKO package builds
- **GitHub Actions** (`.github/workflows/makecommit.yml`): auto-generates syncset artifacts on push
- **CI base image**: `boilerplate:image-v8.3.4` (configured in `.ci-operator.yaml`)

## Linting

`.golangci.yaml` enables only `gosec` with `modules-download-mode: readonly`. The boilerplate also runs its own golangci config. Use `make container-lint` for CI-equivalent results.
