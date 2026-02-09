package rhobs

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordAPIRequest(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		duration  time.Duration
		success   bool
		wantLabel string
	}{
		{
			name:      "successful create_probe",
			operation: "create_probe",
			duration:  100 * time.Millisecond,
			success:   true,
			wantLabel: "success",
		},
		{
			name:      "failed get_probe",
			operation: "get_probe",
			duration:  500 * time.Millisecond,
			success:   false,
			wantLabel: "error",
		},
		{
			name:      "successful delete_probe",
			operation: "delete_probe",
			duration:  200 * time.Millisecond,
			success:   true,
			wantLabel: "success",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := testutil.ToFloat64(apiRequestsTotal.WithLabelValues(tt.operation, tt.wantLabel))
			RecordAPIRequest(tt.operation, tt.duration, tt.success)
			after := testutil.ToFloat64(apiRequestsTotal.WithLabelValues(tt.operation, tt.wantLabel))

			if after != before+1 {
				t.Errorf("expected counter to increment by 1, got delta %f", after-before)
			}
		})
	}
}

func TestRecordOIDCTokenRefresh(t *testing.T) {
	tests := []struct {
		name      string
		duration  time.Duration
		success   bool
		wantLabel string
	}{
		{
			name:      "successful refresh",
			duration:  200 * time.Millisecond,
			success:   true,
			wantLabel: "success",
		},
		{
			name:      "failed refresh",
			duration:  5 * time.Second,
			success:   false,
			wantLabel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := testutil.ToFloat64(oidcTokenRefreshTotal.WithLabelValues(tt.wantLabel))
			RecordOIDCTokenRefresh(tt.duration, tt.success)
			after := testutil.ToFloat64(oidcTokenRefreshTotal.WithLabelValues(tt.wantLabel))

			if after != before+1 {
				t.Errorf("expected counter to increment by 1, got delta %f", after-before)
			}
		})
	}
}

func TestRecordProbeDeletionTimeout(t *testing.T) {
	before := testutil.ToFloat64(probeDeletionTimeoutTotal)
	RecordProbeDeletionTimeout()
	after := testutil.ToFloat64(probeDeletionTimeoutTotal)

	if after != before+1 {
		t.Errorf("expected counter to increment by 1, got delta %f", after-before)
	}
}

func TestSetInfo(t *testing.T) {
	SetInfo("v1.2.3")
	val := testutil.ToFloat64(operatorInfo.WithLabelValues("v1.2.3"))
	if val != 1 {
		t.Errorf("expected info gauge to be 1, got %f", val)
	}
}

func TestMetricsRegistration(t *testing.T) {
	// Verify all metrics are registered with the default registry
	metrics := []prometheus.Collector{
		apiRequestDuration,
		apiRequestsTotal,
		oidcTokenRefreshTotal,
		oidcTokenRefreshDuration,
		probeDeletionTimeoutTotal,
		operatorInfo,
	}

	for _, m := range metrics {
		// Describe returns a channel of metric descriptions; if the metric
		// is properly registered, it should have at least one description
		ch := make(chan *prometheus.Desc, 10)
		m.Describe(ch)
		close(ch)

		count := 0
		for range ch {
			count++
		}
		if count == 0 {
			t.Errorf("metric has no descriptions, may not be registered")
		}
	}
}
