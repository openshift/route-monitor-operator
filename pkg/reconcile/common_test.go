package reconcileCommon_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	// tested package

	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"

	reconcilecommon "github.com/openshift/route-monitor-operator/pkg/reconcile"
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
		mockClient           *clientmocks.MockClient
		mockCtrl             *gomock.Controller
		mockResourceComparer *utilmock.MockResourceComparerInterface

		deepEqual ResourceComparerMockHelper
		get       testhelper.MockHelper
		create    testhelper.MockHelper
		update    testhelper.MockHelper
		delete    testhelper.MockHelper

		rc 	 reconcilecommon.MonitorReconcileCommon
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockResourceComparer = utilmock.NewMockResourceComparerInterface(mockCtrl)

		deepEqual = ResourceComparerMockHelper{}
		get = testhelper.MockHelper{}
		create = testhelper.MockHelper{}
		update = testhelper.MockHelper{}
		delete = testhelper.MockHelper{}

		rc = reconcilecommon.MonitorReconcileCommon{
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
})
