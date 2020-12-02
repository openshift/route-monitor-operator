package int_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestInt(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Int Suite")
}
