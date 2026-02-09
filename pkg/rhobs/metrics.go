package rhobs

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// RHOBS API request metrics
	apiRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rhobs_route_monitor_operator_api_request_duration_seconds",
			Help:    "Duration of RHOBS synthetics API requests",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation", "status"},
	)

	apiRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rhobs_route_monitor_operator_api_requests_total",
			Help: "Total number of RHOBS synthetics API requests",
		},
		[]string{"operation", "status"},
	)

	// OIDC token refresh metrics
	oidcTokenRefreshTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rhobs_route_monitor_operator_oidc_token_refresh_total",
			Help: "Total number of OIDC token refresh attempts",
		},
		[]string{"status"},
	)

	oidcTokenRefreshDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "rhobs_route_monitor_operator_oidc_token_refresh_duration_seconds",
			Help:    "Duration of OIDC token refresh requests",
			Buckets: prometheus.DefBuckets,
		},
	)

	// Probe deletion timeout metric
	probeDeletionTimeoutTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "rhobs_route_monitor_operator_probe_deletion_timeout_total",
			Help: "Total number of probe deletions that exceeded the timeout and triggered fail-open behavior",
		},
	)

	// Operator info metric
	operatorInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rhobs_route_monitor_operator_info",
			Help: "Information about the route-monitor-operator instance",
		},
		[]string{"version"},
	)
)

func init() {
	prometheus.MustRegister(
		apiRequestDuration,
		apiRequestsTotal,
		oidcTokenRefreshTotal,
		oidcTokenRefreshDuration,
		probeDeletionTimeoutTotal,
		operatorInfo,
	)
}

// RecordAPIRequest records metrics for a RHOBS API request.
// Operation should be one of: create_probe, get_probe, delete_probe.
func RecordAPIRequest(operation string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}

	apiRequestDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
	apiRequestsTotal.WithLabelValues(operation, status).Inc()
}

// RecordOIDCTokenRefresh records metrics for an OIDC token refresh attempt.
func RecordOIDCTokenRefresh(duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "error"
	}

	oidcTokenRefreshDuration.Observe(duration.Seconds())
	oidcTokenRefreshTotal.WithLabelValues(status).Inc()
}

// RecordProbeDeletionTimeout increments the counter when a probe deletion
// exceeds the timeout and triggers fail-open behavior.
func RecordProbeDeletionTimeout() {
	probeDeletionTimeoutTotal.Inc()
}

// SetInfo sets the operator info metric with the current version.
func SetInfo(version string) {
	operatorInfo.WithLabelValues(version).Set(1)
}
