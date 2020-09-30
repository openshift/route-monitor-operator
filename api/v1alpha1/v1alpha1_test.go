package v1alpha1_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("V1alpha1", func() {
	var (
		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time
	)
	BeforeEach(func() {
		routeMonitorFinalizers = []string{v1alpha1.FinalizerKey}
		routeMonitorDeletionTimestamp = nil
	})
	JustBeforeEach(func() {
		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers:        routeMonitorFinalizers,
				DeletionTimestamp: routeMonitorDeletionTimestamp,
			},
		}
	})
	Describe("HasFinalizer", func() {
		When("'FinalizerKey' is NOT in the 'Finalizers' list", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
			})

			It("should return false", func() {
				// Act
				res := routeMonitor.HasFinalizer()
				// Assert
				Expect(res).To(BeFalse())
			})
		})
		When("'FinalizerKey' is in the 'Finalizers' list", func() {
			It("should return true", func() {
				// Act
				res := routeMonitor.HasFinalizer()
				// Assert
				Expect(res).To(BeTrue())
			})
		})
	})
	Describe("WasDeleteRequested", func() {
		When("a user Requests a Deletion", func() {
			//Arrange
			BeforeEach(func() {
				routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
			})
			It("should return 'true'", func() {
				// Act
				res := routeMonitor.WasDeleteRequested()
				// Assert
				Expect(res).To(BeTrue())
			})
		})
		When("a user does nothing", func() {
			// Arrange
			It("should return 'false'", func() {
				// Act
				res := routeMonitor.WasDeleteRequested()
				// Assert
				Expect(res).To(BeFalse())
			})
		})
	})
})
