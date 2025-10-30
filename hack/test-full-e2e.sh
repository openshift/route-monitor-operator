#!/bin/bash
# E2E Test for Route Monitor Operator - Mock-based (no cluster required)
# Local repos: set URLs; optionally provide start commands and the script will run them.
#  - RHOBS_API_URL (default http://localhost:8080)
#  - SYNTHETICS_AGENT_URL (default http://localhost:8081)
#  - RHOBS_START_CMD (optional)
#  - SYNTHETICS_AGENT_START_CMD (optional)
# Example:
#  RHOBS_API_URL=http://localhost:19090 \
#  RHOBS_START_CMD='cd /path/to/rhobs && PORT=19090 go run ./cmd/server' \
#  SYNTHETICS_AGENT_URL=http://localhost:19091 \
#  SYNTHETICS_AGENT_START_CMD='cd /path/to/agent && PORT=19091 go run .' \
#  make test-full-e2e

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

cleanup() {
    log_info "Cleaning up mock services..."
    [[ -n "${RHOBS_PID:-}" ]] && kill -TERM $RHOBS_PID 2>/dev/null || true
    [[ -n "${SYNTHETICS_AGENT_PID:-}" ]] && kill -TERM $SYNTHETICS_AGENT_PID 2>/dev/null || true
    sleep 1
    if command -v lsof &> /dev/null; then
        lsof -ti :8080 | xargs kill -9 2>/dev/null || true
        lsof -ti :8081 | xargs kill -9 2>/dev/null || true
    fi
    rm -rf "$PROJECT_ROOT/tmp/mock-*.go"
    log_success "Cleanup completed"
}

trap cleanup EXIT INT TERM

check_prerequisites() {
    command -v go &> /dev/null || { log_error "go not found"; exit 1; }
    export RHOBS_API_URL="${RHOBS_API_URL:-http://localhost:8080}"
    export SYNTHETICS_AGENT_URL="${SYNTHETICS_AGENT_URL:-http://localhost:8081}"
    if [[ "$RHOBS_API_URL" == "http://localhost:8080" ]] && lsof -i :8080 -sTCP:LISTEN >/dev/null 2>&1; then
        log_error "Port 8080 in use. Set RHOBS_API_URL to an unused host:port (e.g. http://localhost:18080)"
        exit 1
    fi
    if [[ "$SYNTHETICS_AGENT_URL" == "http://localhost:8081" ]] && lsof -i :8081 -sTCP:LISTEN >/dev/null 2>&1; then
        log_error "Port 8081 in use. Set SYNTHETICS_AGENT_URL to an unused host:port (e.g. http://localhost:18081)"
        exit 1
    fi
    log_info "Mocks active unless overridden via *_URL or *_START_CMD"
}

start_mock_rhobs_api() {
    if [[ -n "${RHOBS_START_CMD:-}" ]]; then
        log_info "Starting external RHOBS API via RHOBS_START_CMD..."
        bash -lc "$RHOBS_START_CMD" & RHOBS_PID=$!
        sleep 2
        curl -s "$RHOBS_API_URL/api/metrics/v1/test/probes" > /dev/null || { log_error "Failed to reach RHOBS_API_URL after RHOBS_START_CMD"; exit 1; }
        log_success "External RHOBS API started (PID: $RHOBS_PID)"
        return
    fi

    [[ "$RHOBS_API_URL" != "http://localhost:8080" ]] && { log_info "Using external RHOBS API: $RHOBS_API_URL"; return; }
    log_info "Starting mock RHOBS API server..."
    cat > "$PROJECT_ROOT/tmp/mock-rhobs-api.go" << 'EOF'
package main
import ("encoding/json"; "fmt"; "log"; "net/http"; "strings"; "sync"; "time")
type Probe struct{ ID, Status, StaticURL string; Labels map[string]string }
type Server struct{ mu sync.RWMutex; probes map[string]Probe }
func (s *Server) handleProbes(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:
        s.mu.RLock(); defer s.mu.RUnlock()
        labelSel := r.URL.Query().Get("label_selector")
        clusterID := ""; if strings.HasPrefix(labelSel, "cluster-id=") { clusterID = strings.TrimPrefix(labelSel, "cluster-id=") }
        var list []Probe
        if clusterID != "" { if p, ok := s.probes[clusterID]; ok { list = append(list, p) } } else { for _, p := range s.probes { list = append(list, p) } }
        w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{"probes": list})
    case http.MethodPost:
        var in struct{ Labels map[string]string; StaticURL string }
        if err := json.NewDecoder(r.Body).Decode(&in); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
        clusterID := in.Labels["cluster-id"]; if clusterID == "" { http.Error(w, "missing cluster-id label", http.StatusBadRequest); return }
        p := Probe{ID: fmt.Sprintf("probe-%s-%d", clusterID, time.Now().UnixNano()), Labels: in.Labels, Status: "active", StaticURL: in.StaticURL}
        s.mu.Lock(); s.probes[clusterID] = p; s.mu.Unlock()
        w.Header().Set("Content-Type", "application/json"); w.WriteHeader(http.StatusCreated); _ = json.NewEncoder(w).Encode(p)
    default: http.NotFound(w, r) }
}
func (s *Server) handleProbeStatus(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPatch { http.NotFound(w, r); return }
    parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/"); if len(parts) < 1 { http.NotFound(w, r); return }
    id := parts[len(parts)-1]; var body struct{ Status string }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, err.Error(), http.StatusBadRequest); return }
    s.mu.Lock(); for k, p := range s.probes { if p.ID == id { p.Status = body.Status; s.probes[k] = p; break } }; s.mu.Unlock()
    w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
func main() {
    srv := &Server{probes: make(map[string]Probe)}
    http.HandleFunc("/api/metrics/v1/test/probes", srv.handleProbes)
    http.HandleFunc("/api/metrics/v1/test/probes/", srv.handleProbeStatus)
    log.Println("Mock RHOBS API server starting on http://localhost:8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
EOF
    (cd "$PROJECT_ROOT/tmp" && go run ./mock-rhobs-api.go) & RHOBS_PID=$!
    sleep 2; curl -s http://localhost:8080/api/metrics/v1/test/probes > /dev/null || { log_error "Failed to start mock RHOBS API server"; exit 1; }
    log_success "Mock RHOBS API server started (PID: $RHOBS_PID)"
}

start_mock_synthetics_agent() {
    if [[ -n "${SYNTHETICS_AGENT_START_CMD:-}" ]]; then
        log_info "Starting external Synthetics Agent via SYNTHETICS_AGENT_START_CMD..."
        bash -lc "$SYNTHETICS_AGENT_START_CMD" & SYNTHETICS_AGENT_PID=$!
        sleep 2
        curl -s "$SYNTHETICS_AGENT_URL/health" > /dev/null || { log_error "Failed to reach SYNTHETICS_AGENT_URL after start cmd"; exit 1; }
        log_success "External Synthetics Agent started (PID: $SYNTHETICS_AGENT_PID)"
        return
    fi

    log_info "Starting mock Synthetics Agent..."
    cat > "$PROJECT_ROOT/tmp/mock-synthetics-agent.go" << 'EOF'
package main
import ("encoding/json"; "fmt"; "io"; "log"; "net/http"; "net/url"; "os"; "sync"; "time")
type Execution struct{
    ProbeID    string    `json:"probe_id"`
    ClusterID  string    `json:"cluster_id"`
    Status     string    `json:"status"`
    Timestamp  time.Time `json:"timestamp"`
    DurationMs int64     `json:"duration_ms"`
}
type Agent struct{ rhobsURL string; mu sync.RWMutex; probes map[string]map[string]interface{}; executions []Execution }

func (a *Agent) health(w http.ResponseWriter, r *http.Request) {
    a.mu.RLock(); count := len(a.probes); a.mu.RUnlock()
    w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "healthy", "probes": count})
}
func (a *Agent) executionsHandler(w http.ResponseWriter, r *http.Request) {
    q, _ := url.ParseQuery(r.URL.RawQuery)
    clusterID := q.Get("cluster_id")
    a.mu.RLock(); defer a.mu.RUnlock()
    var out []Execution
    for _, e := range a.executions { if clusterID == "" || e.ClusterID == clusterID { out = append(out, e) } }
    w.Header().Set("Content-Type", "application/json"); _ = json.NewEncoder(w).Encode(out)
}
func (a *Agent) poll() {
    a.fetchProbes()
    ticker := time.NewTicker(2 * time.Second)
    defer ticker.Stop()
    for {
        a.fetchProbes()
        <-ticker.C
    }
}
func (a *Agent) fetchProbes() {
    resp, err := http.Get(fmt.Sprintf("%s/api/metrics/v1/test/probes", a.rhobsURL))
    if err != nil { log.Printf("error fetching probes: %v", err); return }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK { b, _ := io.ReadAll(resp.Body); log.Printf("bad status from RHOBS: %d %s", resp.StatusCode, string(b)); return }
    var body struct{ Probes []map[string]interface{} `json:"probes"` }
    if err := json.NewDecoder(resp.Body).Decode(&body); err != nil { log.Printf("decode error: %v", err); return }
    log.Printf("Fetched %d probes from RHOBS", len(body.Probes))
    a.mu.Lock()
    for _, p := range body.Probes {
        labels, _ := p["labels"].(map[string]interface{})
        if labels == nil { continue }
        cid, _ := labels["cluster-id"].(string); if cid == "" { continue }
        if _, exists := a.probes[cid]; !exists {
            a.probes[cid] = p; log.Printf("Executing probe for cluster: %s", cid)
            pid, _ := p["id"].(string); if pid == "" { pid = fmt.Sprintf("probe-%s", cid) }
            a.executions = append(a.executions, Execution{ProbeID: pid, ClusterID: cid, Status: "success", Timestamp: time.Now(), DurationMs: 100})
        }
    }
    a.mu.Unlock()
}
func main() {
    rhobs := os.Getenv("RHOBS_API_URL"); if rhobs == "" { rhobs = "http://localhost:8080" }
    ag := &Agent{rhobsURL: rhobs, probes: make(map[string]map[string]interface{}), executions: make([]Execution, 0)}
    go ag.poll(); http.HandleFunc("/health", ag.health); http.HandleFunc("/probes/executions", ag.executionsHandler)
    log.Println("Mock Synthetics Agent starting on http://localhost:8081")
    log.Fatal(http.ListenAndServe(":8081", nil))
}
EOF
    (cd "$PROJECT_ROOT/tmp" && RHOBS_API_URL="$RHOBS_API_URL" go run ./mock-synthetics-agent.go) & SYNTHETICS_AGENT_PID=$!
    sleep 2; curl -s http://localhost:8081/health > /dev/null || { log_error "Failed to start mock Synthetics Agent"; exit 1; }
    log_success "Mock Synthetics Agent started (PID: $SYNTHETICS_AGENT_PID)"
}

run_e2e_tests() {
    log_info "Running E2E tests..."
    cd "$PROJECT_ROOT"
    go test -v -tags=e2e ./test/e2e/... -timeout=30m || { log_error "E2E tests failed"; exit 1; }
    log_success "E2E tests passed"
}

show_usage() {
    cat << EOF
E2E Test for Route Monitor Operator (Mock-based)

USAGE: $0 [OPTIONS]

OPTIONS:
    -h, --help              Show this help message

ENVIRONMENT VARIABLES:
    RHOBS_API_URL               RHOBS API URL (default: http://localhost:8080)
    SYNTHETICS_AGENT_URL        Synthetics Agent URL (default: http://localhost:8081)
    RHOBS_START_CMD             Command to start your local synthetics-api (checked via RHOBS_API_URL)
    SYNTHETICS_AGENT_START_CMD  Command to start your local synthetics-agent (checked via SYNTHETICS_AGENT_URL)

EXAMPLES:
    # Quick start - run with mocks (no Kubernetes cluster required!)
    $0

    # Test with external services
    RHOBS_API_URL=https://rhobs.example.com \
    SYNTHETICS_AGENT_URL=https://agent.example.com \
    $0

    # Test against local repos by path (examples)
    RHOBS_API_URL=http://localhost:19090 \
    RHOBS_START_CMD='cd /path/to/rhobs && PORT=19090 go run ./cmd/server' \
    SYNTHETICS_AGENT_URL=http://localhost:19091 \
    SYNTHETICS_AGENT_START_CMD='cd /path/to/agent && PORT=19091 go run .' \
    $0

NOTE: No Kubernetes cluster required! Uses mock services for everything.
EOF
}

parse_args() {
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help) show_usage; exit 0 ;;
            *) log_error "Unknown option: $1"; show_usage; exit 1 ;;
        esac
    done
}

main() {
    parse_args "$@"
    log_info "Starting E2E Test for Route Monitor Operator"
    mkdir -p "$PROJECT_ROOT/tmp"
    check_prerequisites
    start_mock_rhobs_api
    start_mock_synthetics_agent
    run_e2e_tests
    log_success "E2E test completed successfully"
}

main "$@"
