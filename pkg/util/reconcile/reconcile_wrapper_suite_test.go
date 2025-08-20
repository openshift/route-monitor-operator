package reconcile_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestReconcileWrapper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ReconcileWrapper Suite")
}

