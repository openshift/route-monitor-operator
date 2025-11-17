#!/bin/bash
set -e

# run-e2e.sh - Full Stack Integration Test Runner
#
# This script runs the complete E2E test that validates integration between:
#   - Route Monitor Operator (RMO)
#   - RHOBS Synthetics API
#   - RHOBS Synthetics Agent
#
# Prerequisites:
#   - Local clones of rhobs-synthetics-api and rhobs-synthetics-agent repos
#   - Either as sibling directories OR via environment variables
#
# What it does:
#   1. Validates RHOBS repo paths (fails fast if missing)
#   2. Runs TestFullStackIntegration with 5 minute timeout
#   3. Reports success/failure
#
# No Docker or Kubernetes cluster required - everything runs as local Go processes!

echo "=============================================="
echo "Route Monitor Operator - Full E2E Test"
echo "=============================================="
echo ""

# Check for required paths
MISSING_PATHS=0

# Check API path
if [ -n "$RHOBS_SYNTHETICS_API_PATH" ]; then
    echo "✓ Using RHOBS Synthetics API from: $RHOBS_SYNTHETICS_API_PATH"
elif [ -d "../rhobs-synthetics-api" ]; then
    echo "✓ Will auto-detect RHOBS Synthetics API at: ../rhobs-synthetics-api"
else
    echo "❌ RHOBS Synthetics API path not set"
    echo "   Set RHOBS_SYNTHETICS_API_PATH or clone to ../rhobs-synthetics-api"
    MISSING_PATHS=1
fi

# Check Agent path
if [ -n "$RHOBS_SYNTHETICS_AGENT_PATH" ]; then
    echo "✓ Using RHOBS Synthetics Agent from: $RHOBS_SYNTHETICS_AGENT_PATH"
elif [ -d "../../rhobs-synthetics-agent" ]; then
    echo "✓ Will auto-detect RHOBS Synthetics Agent at: ../../rhobs-synthetics-agent"
else
    echo "❌ RHOBS Synthetics Agent path not set"
    echo "   Set RHOBS_SYNTHETICS_AGENT_PATH or clone to ../rhobs-synthetics-agent"
    MISSING_PATHS=1
fi

# Exit if paths are missing
if [ $MISSING_PATHS -eq 1 ]; then
    echo ""
    echo "=============================================="
    echo "❌ Missing required RHOBS repositories"
    echo "=============================================="
    echo ""
    echo "Quick setup:"
    echo "  cd .. && git clone https://github.com/rhobs/rhobs-synthetics-api.git"
    echo "  cd .. && git clone https://github.com/rhobs/rhobs-synthetics-agent.git"
    echo "  cd route-monitor-operator && make test-e2e-full"
    echo ""
    exit 1
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

