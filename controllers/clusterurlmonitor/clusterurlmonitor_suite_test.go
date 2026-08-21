package clusterurlmonitor_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClusterurlmonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clusterurlmonitor Suite")
}
