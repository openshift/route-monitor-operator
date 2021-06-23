package reconcileCommon_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	reconcilecommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	utilmock "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/reconcile"
	testhelper "github.com/openshift/route-monitor-operator/pkg/util/test/helper"
)

type ResourceComparerMockHelper struct {
	CalledTimes int
	ReturnValue bool
}

var _ = Describe("CR Deployment Handling", func() {
	var (
		ctx                  context.Context
		mockClient           *clientmocks.MockClient
		mockCtrl             *gomock.Controller
		mockResourceComparer *utilmock.MockResourceComparerInterface

		deepEqual ResourceComparerMockHelper
		get       testhelper.MockHelper
		create    testhelper.MockHelper
		update    testhelper.MockHelper
		delete    testhelper.MockHelper

		rc reconcilecommon.MonitorResourceCommon
	)
	BeforeEach(func() {
		ctx = constinit.Context
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockResourceComparer = utilmock.NewMockResourceComparerInterface(mockCtrl)

		deepEqual = ResourceComparerMockHelper{}
		get = testhelper.MockHelper{}
		create = testhelper.MockHelper{}
		update = testhelper.MockHelper{}
		delete = testhelper.MockHelper{}

		rc = reconcilecommon.MonitorResourceCommon{
			Client:   mockClient,
			Ctx:      ctx,
			Comparer: mockResourceComparer,
		}
	})
	JustBeforeEach(func() {

		mockResourceComparer.EXPECT().DeepEqual(gomock.Any(), gomock.Any()).
			Return(deepEqual.ReturnValue).
			Times(deepEqual.CalledTimes)

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(update.ErrorResponse).
			Times(update.CalledTimes)

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(get.ErrorResponse).
			Times(get.CalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(create.ErrorResponse).
			Times(create.CalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(delete.ErrorResponse).
			Times(delete.CalledTimes)

	})
	AfterEach(func() {
		mockCtrl.Finish()
	})
	Describe("SetErrorStatus", func() {
		var (
			errorStatusString string
		)
		BeforeEach(func() {
			errorStatusString = ""
		})
		When("ErrorStatus is empty and needs to be set", func() {
			It("should set the ErrorStatus and return true", func() {
				res := rc.SetErrorStatus(&errorStatusString, consterror.CustomError)
				Expect(res).To(Equal(true))
				Expect(errorStatusString).To(Equal(consterror.CustomError.Error()))
			})
		})
		When("ErrorStatus was filled but no error has occured", func() {
			BeforeEach(func() {
				errorStatusString = "PreviousErrorState"
			})
			It("should flush the ErrorStatus and return true", func() {
				res := rc.SetErrorStatus(&errorStatusString, nil)
				Expect(res).To(Equal(true))
				Expect(errorStatusString).To(Equal(""))
			})
		})
		When("ErrorStatus was filled and error occured", func() {
			BeforeEach(func() {
				errorStatusString = "PreviousErrorState"
			})
			It("should flush the ErrorStatus and return true", func() {
				res := rc.SetErrorStatus(&errorStatusString, consterror.CustomError)
				Expect(res).To(Equal(false))
				Expect(errorStatusString).To(Equal("PreviousErrorState"))
			})
		})
	})
	Describe("ParseMonitorSLOSpecs", func() {
		var (
			sloSpec v1alpha1.SloSpec
			url     string
			res     string
			err     error
		)
		BeforeEach(func() {
			sloSpec = v1alpha1.SloSpec{}
			url = "test.domain"
			sloSpec.TargetAvailabilityPercent = "99.5"
		})
		JustBeforeEach(func() {
			res, err = rc.ParseMonitorSLOSpecs(url, sloSpec)
		})
		When("the SloSpec is empty", func() {
			BeforeEach(func() {
				sloSpec.TargetAvailabilityPercent = ""
			})
			It("should not return an error as this is a valid setup", func() {
				Expect(res).To(Equal(""))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
		When("the SLO spec contains some wired formated string", func() {
			BeforeEach(func() {
				sloSpec.TargetAvailabilityPercent = "1%!§2"
			})
			It("should return an empty string and an error", func() {
				Expect(res).To(Equal(""))
				Expect(err).To(Equal(customerrors.InvalidSLO))
			})
		})
		When("the url is empty", func() {
			BeforeEach(func() {
				url = ""
			})
			It("should return an empty string and an error", func() {
				Expect(res).To(Equal(""))
				Expect(err).To(Equal(customerrors.NoHost))
			})
		})
		When("the percentage is too high", func() {
			BeforeEach(func() {
				sloSpec.TargetAvailabilityPercent = "102"
			})
			It("should return an empty string and an error", func() {
				Expect(res).To(Equal(""))
				Expect(err).To(Equal(customerrors.InvalidSLO))
			})
		})
		When("the percentage is too low", func() {
			BeforeEach(func() {
				sloSpec.TargetAvailabilityPercent = "-1"
			})
			It("should return an empty string and an error", func() {
				Expect(res).To(Equal(""))
				Expect(err).To(Equal(customerrors.InvalidSLO))
			})
		})
		When("all values are valid", func() {
			It("should return an empty string and an error", func() {
				Expect(res).To(Equal("0.995"))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
	})
	Describe("UpdateMonitorResource", func() {
		var (
			routeMonitor v1alpha1.RouteMonitor
			res          reconcile.Result
			err          error
		)
		BeforeEach(func() {
			routeMonitor = v1alpha1.RouteMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scott-pilgrim",
					Namespace: "the-world",
				},
			}
		})
		JustBeforeEach(func() {
			res, err = rc.UpdateMonitorResource(&routeMonitor)
		})
		When("when updating the monitor failed", func() {
			BeforeEach(func() {
				update.CalledTimes = 1
				update.ErrorResponse = consterror.CustomError
			})
			It("should try to requeue with the particular error", func() {
				Expect(res).To(Equal(reconcile.RequeueOperation()))
				Expect(err).To(Equal(consterror.CustomError))
			})
		})
		When("when updating the monitor succeeds", func() {
			BeforeEach(func() {
				update.CalledTimes = 1
			})
			It("should stop reconciling", func() {
				Expect(res).To(Equal(reconcile.StopOperation()))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
	})
	Describe("SetResourceReference", func() {
		var (
			reference v1alpha1.NamespacedName
			target    types.NamespacedName
			res       bool
			err       error
		)
		BeforeEach(func() {
			reference = v1alpha1.NamespacedName{}
			target = types.NamespacedName{Name: "fake", Namespace: "fake-namespace"}

		})
		JustBeforeEach(func() {
			res, err = rc.SetResourceReference(&reference, target)

		})
		When("when existing reference is flushed", func() {
			BeforeEach(func() {
				reference = v1alpha1.NamespacedName{Name: "fake", Namespace: "fake-namespace"}
				target = types.NamespacedName{}
			})
			It("should indicate that the references has been altered", func() {
				Expect(res).To(Equal(true))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
		When("when empty reference is filled", func() {
			BeforeEach(func() {
				reference = v1alpha1.NamespacedName{}
				target = types.NamespacedName{Name: "fake", Namespace: "fake-namespace"}
			})
			It("should indicate that the references has been altered", func() {
				Expect(res).To(Equal(true))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
		When("when reference is already set according to the target", func() {
			BeforeEach(func() {
				reference = v1alpha1.NamespacedName{Name: "fake", Namespace: "fake-namespace"}
				target = types.NamespacedName{Name: "fake", Namespace: "fake-namespace"}
			})
			It("should indicate that the references has not been altered", func() {
				Expect(res).To(Equal(false))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
		When("it trys to fill reference with new values, which should never happen and is not supported", func() {
			BeforeEach(func() {
				reference = v1alpha1.NamespacedName{Name: "fake", Namespace: "fake-namespace"}
				target = types.NamespacedName{Name: "fake2", Namespace: "fake-namespace2"}
			})
			It("should indicate error", func() {
				Expect(res).To(Equal(false))
				Expect(err).To(Equal(customerrors.InvalidReferenceUpdate))
			})
		})
	})
	Describe("UpdateMonitorResourceStatus", func() {
		var (
			routeMonitor     v1alpha1.RouteMonitor
			res              reconcile.Result
			err              error
			mockStatusWriter *clientmocks.MockStatusWriter
		)
		BeforeEach(func() {
			mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)
			mockClient.EXPECT().Status().Return(mockStatusWriter)

			routeMonitor = v1alpha1.RouteMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scott-pilgrim",
					Namespace: "the-world",
				},
			}
		})
		JustBeforeEach(func() {
			res, err = rc.UpdateMonitorResourceStatus(&routeMonitor)

		})
		When("when updating the monitor failed", func() {
			BeforeEach(func() {
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1).Return(consterror.CustomError)
			})
			It("should try to requeue with the particular error", func() {
				Expect(res).To(Equal(reconcile.RequeueOperation()))
				Expect(err).To(Equal(consterror.CustomError))
			})
		})
		When("when updating the monitor succeeds", func() {
			BeforeEach(func() {
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1)
			})
			It("should stop reconciling", func() {
				Expect(res).To(Equal(reconcile.StopOperation()))
				Expect(err).To(Not(HaveOccurred()))
			})
		})
	})
	Describe("SetFinalizer", func() {
		var (
			routeMonitor v1alpha1.RouteMonitor
			finalizerKey string
			res          bool
		)
		BeforeEach(func() {
			finalizerKey = "theFinalizerKey"
			routeMonitor = v1alpha1.RouteMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "scott-pilgrim",
					Namespace:  "the-world",
					Finalizers: []string{},
				},
			}
		})
		JustBeforeEach(func() {
			res = rc.SetFinalizer(&routeMonitor, finalizerKey)

		})
		When("when the object has already a finalizer set", func() {
			BeforeEach(func() {
				routeMonitor.Finalizers = append(routeMonitor.Finalizers, finalizerKey)
			})
			It("should indicate that the object was changed", func() {
				Expect(res).To(Equal(false))
			})
		})
		When("when the object has no finalizer set", func() {
			It("adds a finalizer and indicates that the object has changed", func() {
				Expect(res).To(Equal(true))
				Expect(routeMonitor.Finalizers[0]).To(Equal(finalizerKey))
			})
		})
	})
	Describe("DeleteFinalizer", func() {
		var (
			routeMonitor v1alpha1.RouteMonitor
			finalizerKey string
			res          bool
		)
		BeforeEach(func() {
			finalizerKey = "theFinalizerKey"
			routeMonitor = v1alpha1.RouteMonitor{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "scott-pilgrim",
					Namespace:  "the-world",
					Finalizers: []string{finalizerKey},
				},
			}
		})
		JustBeforeEach(func() {
			res = rc.DeleteFinalizer(&routeMonitor, finalizerKey)

		})
		When("when the object has a finalizer set", func() {
			It("should delete that one and indicate that the object has changed", func() {
				Expect(res).To(Equal(true))
				Expect(routeMonitor.Finalizers).To(BeEmpty())
			})
		})
		When("when the object has no finalizer set", func() {
			BeforeEach(func() {
				routeMonitor.Finalizers = []string{}
			})
			It("do nothing and indicate that nothing changed", func() {
				Expect(res).To(Equal(false))
			})
		})
	})
})
