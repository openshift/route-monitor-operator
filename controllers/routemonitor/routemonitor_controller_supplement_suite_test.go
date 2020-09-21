package routemonitor_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestRoutemonitor(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Routemonitor Suite")
}
