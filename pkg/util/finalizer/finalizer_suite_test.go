package finalizer_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestFinalizer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Finalizer Suite")
}
