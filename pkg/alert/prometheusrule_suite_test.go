package alert_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPrometheusRule(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prometheus Rule Suite")
}
