package supplement_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestSupplement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Supplement Suite")
}
