package servicemonitor_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestServiceMonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Service Monitor Suite")
}
