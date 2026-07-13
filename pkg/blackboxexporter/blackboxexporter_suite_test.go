package blackboxexporter_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBlackboxexporter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Blackboxexporter Suite")
}
