package servicemonitor_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
	"github.com/openshift/route-monitor-operator/pkg/servicemonitor"

	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	utilmock "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/reconcile"
	testhelper "github.com/openshift/route-monitor-operator/pkg/util/test/helper"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
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

		serviceMonitorRef v1alpha1.NamespacedName
		serviceMonitor    monitoringv1.ServiceMonitor
		sm                servicemonitor.ServiceMonitor
		err               error
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

		serviceMonitorRef = v1alpha1.NamespacedName{}
		serviceMonitor = monitoringv1.ServiceMonitor{}

		sm = servicemonitor.ServiceMonitor{
			Client:   mockClient,
			Ctx:      context.Background(),
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
	Describe("UpdateServiceMonitorDeployment", func() {
		BeforeEach(func() {
			get.CalledTimes = 1
		})
		JustBeforeEach(func() {
			err = sm.UpdateServiceMonitorDeployment(serviceMonitor)
		})
		When("The Client failed to fetch existing deployments", func() {
			BeforeEach(func() {
				get.ErrorResponse = consterror.ErrCustomError
			})
			It("should return the received error", func() {
				Expect(err).To(Equal(consterror.ErrCustomError))
			})
		})
		Describe("No ServiceMonitor has been deployed yet", func() {
			BeforeEach(func() {
				get.ErrorResponse = consterror.NotFoundErr
				create.CalledTimes = 1
			})
			It("tryies to creates one", func() {
				Expect(err).NotTo(HaveOccurred())
			})
			When("an error appeared during the creation", func() {
				BeforeEach(func() {
					create.ErrorResponse = consterror.ErrCustomError
				})
				It("returns the received error", func() {
					Expect(err).To(Equal(consterror.ErrCustomError))
				})
			})
		})
		Describe("A ServiceMonitor has been deployed already", func() {
			BeforeEach(func() {
				deepEqual.CalledTimes = 1
			})
			When("the template changed", func() {
				BeforeEach(func() {
					deepEqual.ReturnValue = false
					update.CalledTimes = 1
				})
				It("updates the existing deployment", func() {
					Expect(err).NotTo(HaveOccurred())
				})
				When("The Client failed to update the existing deployments", func() {
					BeforeEach(func() {
						update.ErrorResponse = consterror.ErrCustomError
					})
					It("should return the received error", func() {
						Expect(err).To(Equal(consterror.ErrCustomError))
					})
				})
			})
			When("the existing ServiceMonitor is equal to the template", func() {
				BeforeEach(func() {
					deepEqual.ReturnValue = true
				})
				It("does nothing", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})
		})
	})
	Describe("DeleteServiceMonitorDeployment", func() {
		JustBeforeEach(func() {
			err = sm.DeleteServiceMonitorDeployment(serviceMonitorRef, false)
		})
		When("The ServiceMonitorRef is not set", func() {
			BeforeEach(func() {
				serviceMonitorRef = v1alpha1.NamespacedName{}
			})
			It("does nothing", func() {
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Describe("The ServiceMonitorRef is set", func() {
			BeforeEach(func() {
				serviceMonitorRef = v1alpha1.NamespacedName{Name: "test", Namespace: "test"}
				get.CalledTimes = 1
			})
			When("the client failed to fetch the deployment", func() {
				BeforeEach(func() {
					get.ErrorResponse = consterror.ErrCustomError
				})
				It("returns the received error", func() {
					Expect(err).To(Equal(consterror.ErrCustomError))
				})
			})
			When("the ServiceMonitorDeployment doesnt exist", func() {
				BeforeEach(func() {
					get.ErrorResponse = consterror.NotFoundErr
				})
				It("does nothing", func() {
					Expect(err).NotTo(HaveOccurred())
				})
			})
			When("the ServiceMonitorDeployment exists", func() {
				BeforeEach(func() {
					delete.CalledTimes = 1
				})
				It("deletes the Deployment", func() {
					Expect(err).NotTo(HaveOccurred())
				})
				When("the client failed to delete the deployment", func() {
					BeforeEach(func() {
						delete.ErrorResponse = consterror.ErrCustomError
					})
					It("returns the received error", func() {
						Expect(err).To(Equal(consterror.ErrCustomError))
					})
				})
			})
		})
	})
})
