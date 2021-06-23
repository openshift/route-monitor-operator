package reconcileCommon_test

import (
	"context"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	// tested package

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"

	reconcilecommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
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
		})
		JustBeforeEach(func() {
			res, err = rc.ParseMonitorSLOSpecs(url, sloSpec)
		})
		When("No SloSpec is empty", func() {
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
				sloSpec.TargetAvailabilityPercent = "99.5"
				url = ""
			})
			It("should return an empty string and an error", func() {
				Expect(res).To(Equal(""))
				Expect(err).To(Equal(customerrors.NoHost))
			})
		})
	})
})
