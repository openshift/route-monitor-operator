package finalizer_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/consts"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/consts"
	"github.com/openshift/route-monitor-operator/pkg/util/finalizer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("Finalizer", func() {
	const (
		key       = "fake-key"
		secondKey = "second-fake-key"
	)
	var (
		list                          []string
		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time
	)
	BeforeEach(func() {
		routeMonitorFinalizers = routemonitorconst.FinalizerList
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
				res := finalizer.HasFinalizer(&routeMonitor, consts.FinalizerKey)
				// Assert
				Expect(res).To(BeFalse())
			})
		})
		When("'FinalizerKey' is in the 'Finalizers' list", func() {
			It("should return true", func() {
				// Act
				res := finalizer.HasFinalizer(&routeMonitor, consts.FinalizerKey)
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
				res := finalizer.WasDeleteRequested(&routeMonitor)
				// Assert
				Expect(res).To(BeTrue())
			})
		})
		When("a user does nothing", func() {
			// Arrange
			It("should return 'false'", func() {
				// Act
				res := finalizer.WasDeleteRequested(&routeMonitor)
				// Assert
				Expect(res).To(BeFalse())
			})
		})
	})
	Describe("Contains", func() {
		When("list is empty and key is defined", func() {
			// Arrange
			BeforeEach(func() {
				list = []string{}
			})
			It("should Return false for a key", func() {
				// Act
				res := finalizer.Contains(list, key)
				// Asset
				Expect(res).To(BeFalse())
			})
		})
		When("list is full and key is not there", func() {
			// Arrange
			BeforeEach(func() {
				list = []string{secondKey}
			})
			It("should Return false for a key isn't in the list", func() {
				// Act
				res := finalizer.Contains(list, key)
				// Asset
				Expect(res).To(BeFalse())
			})
		})
		When("list is full and key is there", func() {
			// Arrange
			BeforeEach(func() {
				list = []string{key}
			})
			It("should Return true for a key is actually there", func() {
				// Act
				res := finalizer.Contains(list, key)
				// Asset
				Expect(res).To(BeTrue())
			})
		})
	})
	Describe("Remove", func() {
		When("object doesnt have the finalizer in Finalizers", func() {
			It("should return an empty list", func() {
				// Arrange
				obj := metav1.ObjectMeta{
					Finalizers: []string{}}
				// Act
				finalizer.Remove(&obj, key)
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{}))
			})
		})
		When("object doesnt have the finalizer in Finalizers", func() {
			It("should return the same list", func() {
				// Arrange
				obj := metav1.ObjectMeta{
					Finalizers: []string{secondKey}}
				// Act
				finalizer.Remove(&obj, key)
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{secondKey}))
			})
		})
		When("key in object fianlizers", func() {
			It("should remove the key", func() {
				// Arrange
				obj := metav1.ObjectMeta{
					Finalizers: []string{key}}
				// Act
				finalizer.Remove(&obj, key)
				// Assert
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{}))
			})
		})
	})
	Describe("Add", func() {
		When("object doesnt have Finalizers", func() {
			It("should create a list", func() {
				// Arrange
				obj := metav1.ObjectMeta{}
				// Act
				finalizer.Add(&obj, key)
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{key}))
			})
		})
		When("object has empty Finalizers", func() {
			It("should create a list", func() {
				// Arrange
				obj := metav1.ObjectMeta{
					Finalizers: []string{}}
				// Act
				finalizer.Add(&obj, key)
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{key}))
			})
		})
		When("object has finalizer in", func() {
			It("should return a bigger", func() {
				// Arrange
				obj := metav1.ObjectMeta{
					Finalizers: []string{secondKey}}
				// Act
				finalizer.Add(&obj, key)
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{key, secondKey}))
			})
		})
		When("key in object fianlizers", func() {
			It("do nothing", func() {
				// Arrange
				obj := metav1.ObjectMeta{
					Finalizers: []string{key}}
				// Act
				finalizer.Add(&obj, key)
				// Assert
				Expect(obj.Finalizers).To(Equal([]string{key}))
			})
		})
	})
})
