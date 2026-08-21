package reconcileCommon_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReconcileCommon(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Reconcile Common Suite")
}
