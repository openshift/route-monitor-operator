# Development Guide

Quick reference for developing the Route Monitor Operator.

## Prerequisites

- **Go**: 1.25.9 or later
- **operator-sdk**: v3 (kubebuilder v3)
- **kubectl**: For cluster interaction
- **prek**: `uv tool install prek`

## Initial Setup

```bash
# Clone repository
git clone https://github.com/openshift/route-monitor-operator.git
cd route-monitor-operator

# Install prek hooks
prek install
```

## Common Commands

### Build
```bash
make go-build                 # Build operator binary
make docker-build             # Build container image
```

### Test
```bash
make test                     # Run all unit tests (excludes int/)
go test ./controllers/routemonitor/...  # Test specific package
ginkgo -v ./controllers/      # Run controller tests with Ginkgo v1
```

### Lint
```bash
make lint                     # Full linting (gosec via golangci-lint)
prek run --all                # Run all prek hooks
prek run golangci-lint        # Lint only
```

### Code Generation
```bash
# After modifying API types (api/v1alpha1/*.go)
make generate

# What this generates:
# - Deepcopy methods (zz_generated.deepcopy.go)
# - OpenAPI schemas
# - CRD manifests (config/crd/bases/)
```

### Run Locally
```bash
# Run against cluster in ~/.kube/config
make run

# Run with debug logging
make run-verbose
```

### Container-based Build
```bash
# Run make targets inside boilerplate container
# (ensures consistent environment with CI)
boilerplate/_lib/container-make
boilerplate/_lib/container-make go-test
boilerplate/_lib/container-make generate
```

## Fast Local Iteration

**Minimal validation loop:**
```bash
# After code changes
go build ./...                # Fast compile check (~5s)
go test ./controllers/routemonitor/  # Run affected tests
prek run                      # Lint staged files
```

**Full validation (pre-PR):**
```bash
prek run --all                # All hooks (~15-30s)
make test                     # Full test suite
```

## Targeted Testing

```bash
# Run specific test (Ginkgo v1 for unit tests)
ginkgo -focus="RouteMonitor" ./controllers/routemonitor/

# Run tests for one package
go test -v ./controllers/routemonitor/

# Run a single test with standard go test
go test ./controllers/routemonitor/... -run TestSpecificName -v

# Skip slow tests during development
ginkgo -skip="E2E" -r ./...
```

## Debugging

```bash
# Verbose operator logs
make run-verbose  # zap-log-level=5

# Print specific package logs
go test -v ./pkg/... 2>&1 | grep "MyFunction"

# Ginkgo verbose output (v1)
ginkgo -v ./...
```

## Dependency Management

```bash
# Add new dependency
go get github.com/some/package@v1.2.3

# Update dependency
go get -u github.com/some/package

# Tidy (removes unused, adds missing)
go mod tidy

# Verify checksums
go mod verify
```

**Note**: `go.sum` changes automatically trigger validation in prek.

## Architecture Pointers

- **API Types**: `api/v1alpha1/` - CRD definitions for RouteMonitor and ClusterUrlMonitor
- **Controllers**: `controllers/{routemonitor,clusterurlmonitor,hostedcontrolplane}/` - Reconciliation logic
- **Core Packages**: `pkg/` - blackboxexporter, servicemonitor, alert, rhobs, dynatrace, reconcile, util
- **Tests**: `*_test.go` alongside source, `*_suite_test.go` for Ginkgo
- **E2E**: `test/e2e/` - End-to-end tests (Ginkgo v2, require deployed operator)
- **Integration**: `int/` - Integration tests requiring live cluster

## CI Parity

Local prek hooks mirror Tekton CI checks:
- **go-check** ↔ Tekton lint job (gosec)
- **go-build** ↔ Compilation in CI
- **go-test** ↔ Unit test job
- **gitleaks** ↔ Security scanning

Run `prek run --all` before pushing to catch CI failures early.

## Boilerplate Integration

This repo uses Red Hat's standardized boilerplate:
- Centralized Makefiles: `boilerplate/openshift/golang-osd-operator/`
- Standard targets: `go-build`, `go-check`, `go-test`
- Container builds: `boilerplate/_lib/container-make`
- Update boilerplate: `make boilerplate-update`

## Troubleshooting

**Prek hook timeout:**
```bash
# macOS: Install GNU timeout
brew install coreutils

# Linux: timeout is built-in
```

**go.sum checksum mismatch:**
```bash
export GOPROXY="https://proxy.golang.org"
go mod tidy
```

**Tests fail locally but pass in CI:**
```bash
# Use container environment
boilerplate/_lib/container-make go-test
```

**Generate target fails:**
```bash
# Use container-make for consistency with CI
boilerplate/_lib/container-make generate
```

## Further Reading

- [Testing Guide](./TESTING.md)
- [Contributing Guide](./CONTRIBUTING.md)
- [Operator SDK Docs](https://sdk.operatorframework.io/)
