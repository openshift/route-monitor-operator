package finalizer_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/consts"
	. "github.com/openshift/route-monitor-operator/pkg/util/finalizer"
)

var _ = Describe("PatchFinalizer", func() {
	const (
		prevFinalizer = "fake-key"
		finalizer     = "second-fake-key"
	)
	var (
		routeMonitor           v1alpha1.RouteMonitor
		routeMonitorFinalizers []string
	)
	BeforeEach(func() {
		routeMonitorFinalizers = routemonitorconst.FinalizerList
	})
	JustBeforeEach(func() {
		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: routeMonitorFinalizers,
			},
		}
	})
	Describe("Patch", func() {
		BeforeEach(func() {
			routeMonitorFinalizers = []string{}
		})
		Describe("noop operations", func() {
			When("'prevFinalizer' and 'finalizer' are not in the finalizer list", func() {
				It("should not change the finalizer list (no patch needed) == the list should stay empty", func() {
					// Act
					Patch(&routeMonitor, prevFinalizer, finalizer)
					// Assert
					Expect(routeMonitor.GetFinalizers()).To(BeEmpty())
				})
			})
		})
		DescribeTable("'finalizer' should be the only output", func(finalizers []string) {
			routeMonitor.ObjectMeta.Finalizers = finalizers
			// Act
			Patch(&routeMonitor, prevFinalizer, finalizer)
			// Assert
			Expect(routeMonitor.GetFinalizers()).To(Equal([]string{finalizer}))
		},
			Entry("only 'finalizer'", []string{finalizer}),
			Entry("only 'prevFinalizer'", []string{prevFinalizer}),
			Entry("both 'prevFinalizer' and 'finalizer'", []string{prevFinalizer, finalizer}),
		)
		Describe("do operation", func() {
			When("'prevFinalizer' and 'finalizer' are in the finalizer list", func() {
				BeforeEach(func() {
					routeMonitorFinalizers = []string{prevFinalizer, finalizer}
				})
				It("should remove the 'prevFinalizer' from the list (patch occured) == the list should stay only with 'finalizer'", func() {
					// Act
					Patch(&routeMonitor, prevFinalizer, finalizer)
					// Assert
					Expect(routeMonitor.GetFinalizers()).To(Equal([]string{finalizer}))
				})
			})
			When("'finalizer' is in the finalizer list alone", func() {
				BeforeEach(func() {
					routeMonitorFinalizers = []string{finalizer}
				})
				It("should not change the finalizer list (no patch needed) == the list should stay only with 'finalizer'", func() {
					// Act
					Patch(&routeMonitor, prevFinalizer, finalizer)
					// Assert
					Expect(routeMonitor.GetFinalizers()).To(Equal([]string{finalizer}))
				})
			})
		})
	})

})
