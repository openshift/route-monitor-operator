package blackboxexporter_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestBlackBoxExporter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BlackBoxExporter Suite")
}
