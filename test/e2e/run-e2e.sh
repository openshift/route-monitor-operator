#!/bin/bash
set -e

# Full Stack Integration Test Runner for Route Monitor Operator
# Tests RMO → RHOBS Synthetics API → Agent integration
# No Docker required - everything runs as Go processes!

echo "=============================================="
echo "Route Monitor Operator - Full E2E Test"
echo "=============================================="
echo ""

# Show which sources are being used
if [ -n "$RHOBS_SYNTHETICS_API_PATH" ]; then
    echo "✓ Using local RHOBS Synthetics API from: $RHOBS_SYNTHETICS_API_PATH"
else
    echo "✓ Using RHOBS Synthetics API from GitHub (via Go modules)"
fi

if [ -n "$RHOBS_SYNTHETICS_AGENT_PATH" ]; then
    echo "✓ Using local RHOBS Synthetics Agent from: $RHOBS_SYNTHETICS_AGENT_PATH"
else
    echo "✓ Using RHOBS Synthetics Agent from GitHub (via Go modules)"
fi

echo ""
echo "Running full integration test..."
echo ""

# Run the test with appropriate timeout
# Note: We're already in test/e2e directory, so test current directory
go test -v -tags=e2e -timeout=5m . -run TestFullStackIntegration

echo ""
echo "=============================================="
echo "✅ All tests passed!"
echo "=============================================="

