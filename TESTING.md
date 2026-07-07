# Testing Guide

Testing guidelines for the Route Monitor Operator.

## Framework

This repository uses two versions of Ginkgo:

- **Ginkgo v1** (`github.com/onsi/ginkgo v1.16.5`): BDD testing framework used in unit/controller tests
- **Ginkgo v2** (`github.com/onsi/ginkgo/v2 v2.27.3`): Used in E2E tests (`test/e2e/`)
- **Gomega**: Matchers and assertions (used with both Ginkgo versions)

There is **no envtest** in this repository. Tests use real, fake, or generated mock implementations (via `//go:generate mockgen`; generated mocks in `pkg/util/test/generated/mocks/`) alongside Ginkgo/Gomega.

## Quick Commands

```bash
# Run all unit tests (standard target)
make test

# Run tests with Ginkgo v1 runner
ginkgo -r ./...

# Run specific package
go test ./controllers/routemonitor/

# Verbose output
ginkgo -v ./...

# Run focused test (Ginkgo v1)
ginkgo -focus="RouteMonitor" ./controllers/routemonitor/

# Run a single test with standard go test
go test ./controllers/routemonitor/... -run TestSpecificName -v

# Container-based (CI parity)
boilerplate/_lib/container-make go-test

# E2E tests (Ginkgo v2, require deployed operator)
ginkgo -r ./test/e2e/
```

## Writing Tests

### Test Structure

Each package with tests includes:
- `*_suite_test.go`: Ginkgo test suite setup (v1)
- `*_test.go`: Actual test cases

**Example (Ginkgo v1 - unit tests):**
```go
package routemonitor_test

import (
    . "github.com/onsi/ginkgo"
    . "github.com/onsi/gomega"
)

var _ = Describe("RouteMonitor", func() {
    Context("when reconciling a RouteMonitor", func() {
        It("should create a ServiceMonitor", func() {
            result := reconciler.Reconcile(ctx, req)
            Expect(result).NotTo(HaveOccurred())
        })
    })
})
```

**Example (Ginkgo v2 - E2E tests in test/e2e/):**
```go
package e2e_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("RouteMonitor E2E", func() {
    It("should probe routes and generate alerts", func() {
        // End-to-end validation
    })
})
```

### Bootstrapping Tests

```bash
# For v1 (unit tests)
cd controllers/routemonitor
ginkgo bootstrap   # Creates suite file
ginkgo generate routemonitor_controller.go  # Creates test file
```

## Test Organization

### Unit Tests
- Test individual functions and methods
- Use Ginkgo v1 BDD style
- No envtest — no simulated Kubernetes API server
- Fast execution (<1s per package)
- Located alongside source code (`controllers/`, `pkg/`)

### Controller Tests
- Test reconciliation logic using Ginkgo v1
- Use fake clients or minimal stubs for Kubernetes API interactions
- Test RouteMonitor and ClusterUrlMonitor resource lifecycle
- Located in `controllers/routemonitor/`, `controllers/clusterurlmonitor/`, `controllers/hostedcontrolplane/`

### Integration Tests
- Located in `int/` (excluded from `make test`)
- Require a live cluster and `oc login`
- Run via `hack/test-integration.sh`

### E2E Tests
- Located in `test/e2e/`
- Use Ginkgo v2
- Require `ginkgo` binary and a deployed operator
- Build with `--tags=osde2e`
- Run in CI via separate Tekton e2e pipelines

## CRD Examples in Tests

### RouteMonitor
```go
routeMonitor := &v1alpha1.RouteMonitor{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "test-route-monitor",
        Namespace: "openshift-route-monitor-operator",
    },
    Spec: v1alpha1.RouteMonitorSpec{
        Route: v1alpha1.RouteMonitorRouteSpec{
            Name:      "my-route",
            Namespace: "my-namespace",
        },
        SloSpec: v1alpha1.SloSpec{
            TargetAvailabilityPercent: "99.9",
        },
    },
}
```

### ClusterUrlMonitor
```go
clusterUrlMonitor := &v1alpha1.ClusterUrlMonitor{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "test-cluster-url-monitor",
        Namespace: "openshift-route-monitor-operator",
    },
    Spec: v1alpha1.ClusterUrlMonitorSpec{
        Prefix: "https://",
        Port:   "443",
        Suffix: "/healthz",
        SloSpec: v1alpha1.SloSpec{
            TargetAvailabilityPercent: "99.5",
        },
    },
}
```

## Agent-Driven Validation

When AI agents modify code:

**Minimal validation:**
```bash
# After changing controllers/routemonitor/
go test ./controllers/routemonitor/
```

**Full validation before commit:**
```bash
make test
```

**If tests fail:**
1. Read test output carefully
2. Fix the underlying issue (don't skip tests)
3. Rerun to confirm fix
4. Regenerate if API types changed: `make generate`

## Common Patterns

### Testing Controllers

```go
It("should reconcile resource", func() {
    // Setup: create the reconciler and resource
    reconciler := &RouteMonitorReconciler{...}

    // Trigger reconciliation
    _, err := reconciler.Reconcile(ctx, req)
    Expect(err).NotTo(HaveOccurred())
})
```

### Testing Error Conditions

```go
It("should return error when resource not found", func() {
    _, err := reconciler.Reconcile(ctx, reqForNonExistent)
    Expect(err).To(HaveOccurred())
})
```

### Using Matchers

```go
// Equality
Expect(result).To(Equal(expected))

// Nil checks
Expect(err).NotTo(HaveOccurred())
Expect(obj).To(BeNil())

// Collections
Expect(slice).To(ContainElement("item"))
Expect(slice).To(HaveLen(3))
Expect(slice).To(BeEmpty())

// Booleans
Expect(condition).To(BeTrue())
Expect(condition).To(BeFalse())

// Eventually (async)
Eventually(func() bool {
    return checkCondition()
}).Should(BeTrue())
```

## Coverage

Generate coverage report:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

**Note**: Aim for meaningful coverage, not arbitrary percentages.
- Test critical paths and error handling
- Don't test generated code or trivial getters/setters

## Debugging Tests

```bash
# Verbose Ginkgo output (v1)
ginkgo -v ./...

# Print statements in tests
fmt.Fprintf(GinkgoWriter, "Debug: %v\n", value)

# Skip flaky tests temporarily (v1)
ginkgo -skip="FlakyTest" ./...

# Run single test
ginkgo -focus="exact test name" ./...

# Race detector
go test -race ./controllers/routemonitor/
```

## CI Expectations

Tests run in Tekton pipeline with:
- Fresh environment
- No cached dependencies
- Strict timeout limits

**Local CI parity:**
```bash
boilerplate/_lib/container-make go-test
```

## Test Performance

**Target timings:**
- Unit tests: <5s per package
- Controller tests: <15s per controller
- Full suite: <2min
- E2E: varies (requires live cluster)

**If tests are slow:**
- Check for unnecessary sleeps
- Use `Eventually` with shorter intervals
- Avoid creating unnecessary Kubernetes resources

## Common Issues

**Tests pass locally, fail in CI:**
```bash
# Run in container environment
boilerplate/_lib/container-make go-test

# Check for:
# - Time-dependent tests
# - Environment-specific assumptions
# - File path dependencies
```

**Flaky tests:**
- Use `Eventually` instead of `Expect` for async operations
- Avoid hardcoded delays
- Ensure test isolation (clean up resources)

## Prek Integration

Tests do not run automatically in prek hooks (too slow for interactive use).
Run manually before pushing: `make test`

## Further Reading

- [Ginkgo v1 Documentation](https://onsi.github.io/ginkgo/v1/)
- [Ginkgo v2 Documentation](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [TESTS.md](./TESTS.md) - Legacy test overview
