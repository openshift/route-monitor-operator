package adder_test

import (
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor/adder"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	consterror "github.com/openshift/route-monitor-operator/pkg/const/test/error"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
)

var _ = Describe("Adder", func() {
	var (
		scheme = constinit.Scheme
	)
	var (
		mockClient *clientmocks.MockClient
		mockCtrl   *gomock.Controller

		routeMonitorAdder       adder.RouteMonitorAdder
		routeMonitorAdderClient client.Client

		ctx context.Context

		routeMonitor           v1alpha1.RouteMonitor
		routeMonitorStatus     v1alpha1.RouteMonitorStatus
		routeMonitorFinalizers []string

		getCalledTimes      int
		getErrorResponse    error
		deleteCalledTimes   int
		deleteErrorResponse error
		createCalledTimes   int
		createErrorResponse error
		updateCalledTimes   int
		updateErrorResponse error
	)
	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)

		routeMonitorAdderClient = mockClient

		ctx = constinit.Context

		getCalledTimes = 0
		getErrorResponse = nil
		deleteCalledTimes = 0
		deleteErrorResponse = nil
		createCalledTimes = 0
		createErrorResponse = nil
		updateCalledTimes = 0
		updateErrorResponse = nil

		routeMonitorAdderClient = mockClient
		routeMonitorStatus = v1alpha1.RouteMonitorStatus{
			RouteURL: "fake-route-url",
		}
		routeMonitorFinalizers = routemonitorconst.FinalizerList

	})
	JustBeforeEach(func() {
		routeMonitorAdder = adder.RouteMonitorAdder{
			Log:    constinit.Logger,
			Client: routeMonitorAdderClient,
			Scheme: constinit.Scheme,
		}

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(updateErrorResponse).
			Times(updateCalledTimes)

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(getErrorResponse).
			Times(getCalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(deleteErrorResponse).
			Times(deleteCalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(createErrorResponse).
			Times(createCalledTimes)

		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Finalizers: routeMonitorFinalizers,
			},
			Status: routeMonitorStatus,
		}
	})
	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("Testing CreateResourceIfNotFound", func() {
		BeforeEach(func() {
			// Arrange
			getCalledTimes = 1
			getErrorResponse = consterror.NotFoundErr
			createCalledTimes = 1
		})
		When("the resource Exists", func() {
			BeforeEach(func() {
				// Arrange
				getCalledTimes = 1
				getErrorResponse = nil
				createCalledTimes = 0
			})
			It("should call `Get` and not call `Create`", func() {
				//Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				//Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the resource Get fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.CustomError
				createCalledTimes = 0
			})
			It("should return the error and not call `Create`", func() {
				//Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(getErrorResponse))
			})
		})

		When("the resource is Not Found", func() {
			// Arrange
			It("should call `Get` successfully and `Create` the resource", func() {
				//Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				//Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("the resource Create fails unexpectedly", func() {
			BeforeEach(func() {
				// Arrange
				createErrorResponse = consterror.CustomError
			})
			It("should call `Get` Successfully and call `Create` but return the error", func() {
				//Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})

	})
	Describe("CreateBlackBoxExporterDeployment", func() {
		BeforeEach(func() {
			routeMonitorAdderClient = mockClient
			// Arrange
			getCalledTimes = 1

		})

		When("the resource(deployment) Exists", func() {
			It("should call `Get` and not call `Create`", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterDeploymentExists(ctx)
				//Assert
				Expect(err).NotTo(HaveOccurred())

			})
		})
		When("the resource(deployment) is Not Found", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.NotFoundErr
				createCalledTimes = 1
			})
			It("should call `Get` successfully and `Create` the resource(deployment)", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterDeploymentExists(ctx)
				//Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the resource(deployment) Get fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.CustomError
			})
			It("should return the error and not call `Create`", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterDeploymentExists(ctx)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(getErrorResponse))
			})
		})
		When("the resource(deployment) Create fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = consterror.NotFoundErr
				createCalledTimes = 1
				createErrorResponse = consterror.CustomError
			})
			It("should call `Get` Successfully and call `Create` but return the error", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterDeploymentExists(ctx)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(createErrorResponse))
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
	})
	Describe("CreateBlackBoxExporterService", func() {
		BeforeEach(func() {
			routeMonitorAdderClient = mockClient
		})

		When("the resource(service) Exists", func() {
			// Arrange
			BeforeEach(func() {
				getCalledTimes = 1
			})
			It("should call `Get` and not call `Create`", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterServiceExists(ctx)
				//Assert
				Expect(err).NotTo(HaveOccurred())

			})
		})
		When("the resource(service) is Not Found", func() {
			// Arrange
			BeforeEach(func() {
				getCalledTimes = 1
				getErrorResponse = consterror.NotFoundErr
				createCalledTimes = 1
			})
			It("should call `Get` successfully and `Create` the resource(service)", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterServiceExists(ctx)
				//Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the resource(service) Get fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				getCalledTimes = 1
				getErrorResponse = consterror.CustomError
			})
			It("should return the error and not call `Create`", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterServiceExists(ctx)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(getErrorResponse))
			})
		})
		When("the resource(service) Create fails unexpectedly", func() {
			// Arrange
			BeforeEach(func() {
				getCalledTimes = 1
				getErrorResponse = consterror.NotFoundErr
				createCalledTimes = 1
				createErrorResponse = consterror.CustomError
			})
			It("should call `Get` Successfully and call `Create` but return the error", func() {
				//Act
				err := routeMonitorAdder.EnsureBlackBoxExporterServiceExists(ctx)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(createErrorResponse))
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
	})
	Describe("CreateServiceMonitorResource", func() {
		When("the RouteMonitor has no Host", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorAdderClient = fake.NewFakeClientWithScheme(scheme)
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{}
			})
			It("should return No Host error", func() {
				// Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.NoHost))
			})
		})
		When("func 'Update' failed unexpectidly", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorAdderClient = mockClient
				updateCalledTimes = 1
				updateErrorResponse = consterror.CustomError
				routeMonitorFinalizers = nil
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
			})
			It("should bubble up the error", func() {
				// Act
				_, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(consterror.CustomError))
			})
		})
		When("the RouteMonitor has no Finalizer", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorAdderClient = mockClient
				updateCalledTimes = 1
				routeMonitorFinalizers = nil
				routeMonitorStatus = v1alpha1.RouteMonitorStatus{
					RouteURL: "fake-route-url",
				}
			})
			It("Should update the RouteMonitor with the finalizer", func() {
				// Act
				resp, err := routeMonitorAdder.EnsureServiceMonitorResourceExists(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})

	Describe("New", func() {
		When("func New is called", func() {
			It("should return a new Deleter object", func() {
				// Arrange
				r := routemonitor.RouteMonitorReconciler{
					Client: routeMonitorAdderClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}
				// Act
				res := adder.New(r)
				// Assert
				Expect(res).To(Equal(&adder.RouteMonitorAdder{
					Client: routeMonitorAdderClient,
					Log:    constinit.Logger,
					Scheme: constinit.Scheme,
				}))
			})
		})
	})
})
