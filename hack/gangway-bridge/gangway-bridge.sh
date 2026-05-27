#!/bin/bash
set -o nounset
set -o errexit
set -o pipefail

GANGWAY_URL="https://gangway-ci.apps.ci.l2s4.p1.openshiftapps.com/v1/executions"
POLL_INTERVAL="${POLL_INTERVAL:-60}"
TIMEOUT="${TIMEOUT:-3600}"
CURL_CONNECT_TIMEOUT=10
CURL_MAX_TIME=30
MAX_RETRIES=3
RETRY_WAIT=10

log() {
    echo -e "\033[1m$(date '+%Y-%m-%dT%H:%M:%S') ${*}\033[0m" >&2
}

if [[ -z "${JOB_NAME:-}" ]]; then
    log "ERROR: JOB_NAME is required"
    exit 1
fi

if [[ -z "${GANGWAY_TOKEN:-}" ]]; then
    log "ERROR: GANGWAY_TOKEN is required"
    exit 1
fi

log "Triggering Prow job: ${JOB_NAME}"
log "  Poll interval: ${POLL_INTERVAL}s"
log "  Timeout: ${TIMEOUT}s"

# Build the request body with optional env overrides.
# JOB_ENVS is a comma-separated list of KEY=VALUE pairs passed to the Prow job.
build_request_body() {
    if [[ -n "${JOB_ENVS:-}" ]]; then
        local envs_json="{}"
        IFS=',' read -ra ENV_LIST <<< "${JOB_ENVS}"
        for entry in "${ENV_LIST[@]}"; do
            local key="${entry%%=*}"
            local value="${entry#*=}"
            envs_json=$(echo "${envs_json}" | jq --arg k "${key}" --arg v "${value}" '. + {($k): $v}')
        done
        jq -cn --argjson envs "${envs_json}" '{"job_execution_type": "1", "pod_spec_options": {"envs": $envs}}'
    else
        echo '{"job_execution_type": "1"}'
    fi
}

REQUEST_BODY=$(build_request_body)
if [[ -n "${JOB_ENVS:-}" ]]; then
    log "  Env overrides: $(echo "${JOB_ENVS}" | sed 's/=[^,]*/=***/g')"
fi

# Trigger the Prow job via Gangway API
trigger_job() {
    local response http_code body
    for attempt in $(seq 1 "${MAX_RETRIES}"); do
        response=$(curl -sSL --connect-timeout "${CURL_CONNECT_TIMEOUT}" --max-time "${CURL_MAX_TIME}" \
            -w "\n%{http_code}" \
            -X POST \
            -H "Authorization: Bearer ${GANGWAY_TOKEN}" \
            -H "Content-Type: application/json" \
            -d "${REQUEST_BODY}" \
            "${GANGWAY_URL}/${JOB_NAME}" 2>/dev/null) || true

        http_code=$(echo "${response}" | tail -1)
        body=$(echo "${response}" | sed '$d')

        if [[ "${http_code}" == "200" ]]; then
            echo "${body}"
            return 0
        fi

        log "Trigger attempt ${attempt}/${MAX_RETRIES} failed (HTTP ${http_code})"
        if [[ ${attempt} -lt ${MAX_RETRIES} ]]; then
            sleep ${RETRY_WAIT}
        fi
    done

    log "ERROR: Failed to trigger job after ${MAX_RETRIES} attempts (HTTP ${http_code})"
    return 1
}

# Query job status via Gangway API
query_status() {
    local job_id="$1"
    curl -sSL --connect-timeout "${CURL_CONNECT_TIMEOUT}" --max-time "${CURL_MAX_TIME}" \
        -H "Authorization: Bearer ${GANGWAY_TOKEN}" \
        "${GANGWAY_URL}/${job_id}" 2>/dev/null
}

# Trigger
TRIGGER_RESPONSE=$(trigger_job)
JOB_ID=$(echo "${TRIGGER_RESPONSE}" | jq -r '.id // empty' 2>/dev/null || true)

if [[ -z "${JOB_ID}" ]]; then
    log "ERROR: Could not extract job ID from trigger response"
    exit 1
fi

log "Job triggered successfully"
log "  Job ID: ${JOB_ID}"
log "  Prow: https://prow.ci.openshift.org/view/gs/test-platform-results/logs/${JOB_NAME}/${JOB_ID}"

# Poll for completion
START_TIME=$(date +%s)

while true; do
    ELAPSED=$(( $(date +%s) - START_TIME ))
    if [[ ${ELAPSED} -ge ${TIMEOUT} ]]; then
        log "ERROR: Timeout after ${TIMEOUT}s waiting for job ${JOB_ID}"
        exit 1
    fi

    sleep "${POLL_INTERVAL}"

    STATUS_RESPONSE=$(query_status "${JOB_ID}" || echo '{}')
    JOB_STATUS=$(echo "${STATUS_RESPONSE}" | jq -r '.job_status // "UNKNOWN"' 2>/dev/null || echo "UNKNOWN")

    ELAPSED=$(( $(date +%s) - START_TIME ))
    log "Status: ${JOB_STATUS} (${ELAPSED}s elapsed)"

    case "${JOB_STATUS}" in
        SUCCESS)
            log "Job ${JOB_ID} succeeded"
            exit 0
            ;;
        FAILURE|ABORTED|ERROR)
            log "Job ${JOB_ID} failed with status: ${JOB_STATUS}"
            exit 1
            ;;
        PENDING|TRIGGERED|UNKNOWN)
            continue
            ;;
        *)
            log "Unknown status: ${JOB_STATUS}, continuing to poll"
            ;;
    esac
done
