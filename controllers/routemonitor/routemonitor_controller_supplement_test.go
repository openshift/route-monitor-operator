package routemonitor_test

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr/testing"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	mocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks"
)

var _ = Describe("Routemonitor", func() {
	var (
		routeMonitor          monitoringv1alpha1.RouteMonitor
		routeMonitorName      string
		routeMonitorNamespace string
		routeMonitorRouteSpec monitoringv1alpha1.RouteMonitorRouteSpec

		routeMonitorReconciler       routemonitor.RouteMonitorReconciler
		routeMonitorReconcilerClient client.Client

		req                  ctrl.Request
		route                routev1.Route
		expectedRouteMonitor monitoringv1alpha1.RouteMonitor

		mockClient       *mocks.MockClient
		mockStatusWriter *mocks.MockStatusWriter
		mockCtrl         *gomock.Controller

		// createCalledTimes it a toggle for the 'mockClient.EXPECT.Create()'. If set to 0 then ignored
		createCalledTimes   int
		createErrorResponse error
		// getCalledTimes it a toggle for the 'mockClient.EXPECT.Get()'. If set to 0 then ignored
		getCalledTimes   int
		getErrorResponse error
		// updateCalledTimes it a toggle for the 'mockClient.EXPECT.Get()'. If set to 0 then ignored
		updateCalledTimes   int
		updateErrorResponse error
	)

	const (
		routeMonitorStatusRouteURL = "fake-route-url"
	)

	var ( // Practically const vars
		ctx         = context.TODO()
		logger      = testing.NullLogger{}
		scheme      = setScheme(runtime.NewScheme())
		notFoundErr = &k8serrors.StatusError{
			ErrStatus: metav1.Status{
				Reason: metav1.StatusReasonNotFound,
			}}
		customError = errors.New("test")
	)
	BeforeEach(func() {
		routeMonitorName = "fake-name"
		routeMonitorNamespace = "fake-namespace"
		routeMonitorRouteSpec = monitoringv1alpha1.RouteMonitorRouteSpec{}
		routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme)

		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = mocks.NewMockClient(mockCtrl)
		mockStatusWriter = mocks.NewMockStatusWriter(mockCtrl)
		createCalledTimes = 0
		createErrorResponse = nil
		getCalledTimes = 0
		getErrorResponse = nil
		updateCalledTimes = 0
		updateErrorResponse = nil

	})

	JustBeforeEach(func() {
		routeMonitor = monitoringv1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:       routeMonitorName,
				Namespace:  routeMonitorNamespace,
				Finalizers: []string{routemonitor.FinalizerKey},
			},
			Spec: monitoringv1alpha1.RouteMonitorSpec{
				Route: routeMonitorRouteSpec,
			},
		}
		routeMonitorReconciler = routemonitor.RouteMonitorReconciler{
			Log:    logger,
			Client: routeMonitorReconcilerClient,
			Scheme: scheme,
		}
		req = ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      routeMonitorName,
				Namespace: routeMonitorNamespace,
			},
		}
		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(getErrorResponse).
			Times(getCalledTimes)

		mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).
			Return(updateErrorResponse).
			Times(updateCalledTimes)

		mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
			Return(createErrorResponse).
			Times(createCalledTimes)
		expectedRouteMonitor = routeMonitor
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})
	Describe("GetRouteMonitor", func() {
		When("the RouteMonitor is not found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})
			It("should return Not Found Error", func() {
				// Act
				resRouteMonitor, err := routeMonitorReconciler.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(resRouteMonitor).To(BeNil())
			})
		})
		When("the RouteMonitor is found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})

			It("should return the object", func() {
				// Act
				resRouteMonitor, err := routeMonitorReconciler.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).ToNot(HaveOccurred())
				Expect(resRouteMonitor).ToNot(BeNil())
			})
		})
	})
	Describe("GetRoute", func() {
		When("the Route is not found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorRouteSpec = monitoringv1alpha1.RouteMonitorRouteSpec{
					Name:      "Rob",
					Namespace: "Bob",
				}
				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "testns",
					},
				}
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &route)
			})
			It("should return a Not Found error", func() {
				// Act
				resRoute, err := routeMonitorReconciler.GetRoute(ctx, &routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(resRoute).To(BeNil())

			})
		})
		When("the Route is found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorRouteSpec = monitoringv1alpha1.RouteMonitorRouteSpec{
					Name:      routeMonitorName,
					Namespace: routeMonitorNamespace,
				}

				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeMonitorName,
						Namespace: routeMonitorNamespace,
					},
				}
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &route)
			})
			It("should return the route", func() {
				// Act
				resRoute, err := routeMonitorReconciler.GetRoute(ctx, &routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resRoute).ShouldNot(BeNil())
			})
		})

		Describe("Missing a RouteMonitor Field", func() {
			When("the RouteMonitor doesnt have Spec.Route.Namespace", func() {
				// Arrange
				BeforeEach(func() {
					routeMonitorRouteSpec = monitoringv1alpha1.RouteMonitorRouteSpec{
						Name: "Rob",
						// Namespace is omitted
					}

				})
				It("should return a custom error", func() {
					// Act
					resRoute, err := routeMonitorReconciler.GetRoute(ctx, &routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeNil())
				})
			})
			When("the RouteMonitor doesnt have Spec.Route.Name", func() {
				// Arrange
				BeforeEach(func() {
					routeMonitorRouteSpec = monitoringv1alpha1.RouteMonitorRouteSpec{
						// Name is omitted
						Namespace: "Bob",
					}
				})
				It("should return a custom error", func() {
					// Act
					resRoute, err := routeMonitorReconciler.GetRoute(ctx, &routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeNil())
				})
			})
		})
	})
	Describe("UpdateRouteURL", func() {
		BeforeEach(func() {
			route = routev1.Route{}
		})
		When("the Route has no Ingresses", func() {
			// Arrange
			It("should return No Ingress error", func() {
				// Act
				res, err := routeMonitorReconciler.UpdateRouteURL(ctx, &route, &routeMonitor)
				// Assert
				Expect(res).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("No Ingress:"))
			})
		})
		When("the Route has no Host", func() {
			// Arrange
			BeforeEach(func() {
				route.Status = routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{Host: ""},
					},
				}
			})
			It("should return No Host error", func() {
				// Act
				res, err := routeMonitorReconciler.UpdateRouteURL(ctx, &route, &routeMonitor)
				// Assert
				Expect(res).To(BeNil())
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.NoHost))
			})
		})
		When("the Route has too many Ingress", func() {
			// Arrange
			var (
				firstRouteURL = "freddy"
			)
			BeforeEach(func() {
				route.Status = routev1.RouteStatus{
					Ingress: []routev1.RouteIngress{
						{Host: firstRouteURL},
						{Host: "eddie"},
					},
				}
				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				routeMonitorReconcilerClient = mockClient

			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Eq(&expectedRouteMonitor))

			})
			It("should update the first ingress", func() {
				// Act
				res, err := routeMonitorReconciler.UpdateRouteURL(ctx, &route, &routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(&ctrl.Result{Requeue: true}))
			})
		})
	})
	Describe("CreateServiceMonitor", func() {
		When("the RouteMonitor has no Host", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme)
			})
			It("should return No Host error", func() {
				// Act
				_, err := routeMonitorReconciler.CreateServiceMonitor(ctx, &routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customerrors.NoHost))
			})
		})
		When("the RouteMonitor has no Finalizer", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				updateCalledTimes = 1
			})
			JustBeforeEach(func() {
				routeMonitor.ObjectMeta.Finalizers = nil
				routeMonitor.Status.RouteURL = routeMonitorStatusRouteURL
			})
			It("Should update the RouteMonitor with the finalizer", func() {
				// Act
				resp, err := routeMonitorReconciler.CreateServiceMonitor(ctx, &routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp.Requeue).To(Equal(true))

			})

		})
		Describe("Testing CreateResourceIfNotFound", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
			})
			JustBeforeEach(func() {
				routeMonitor.Status.RouteURL = routeMonitorStatusRouteURL
			})

			When("the resource Exists", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
				})
				It("should call `Get` and not call `Create`", func() {
					//Act
					_, err := routeMonitorReconciler.CreateServiceMonitor(ctx, &routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())

				})
			})
			When("the resource is Not Found", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = notFoundErr
					createCalledTimes = 1
				})
				It("should call `Get` successfully and `Create` the resource", func() {
					//Act
					_, err := routeMonitorReconciler.CreateServiceMonitor(ctx, &routeMonitor)
					//Assert
					Expect(err).NotTo(HaveOccurred())
				})
			})
			When("the resource Get fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = customError
				})
				It("should return the error and not call `Create`", func() {
					//Act
					_, err := routeMonitorReconciler.CreateServiceMonitor(ctx, &routeMonitor)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(getErrorResponse))
				})
			})
			When("the resource Create fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = notFoundErr
					createCalledTimes = 1
					createErrorResponse = customError
				})
				It("should call `Get` Successfully and call `Create` but return the error", func() {
					//Act
					_, err := routeMonitorReconciler.CreateServiceMonitor(ctx, &routeMonitor)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(createErrorResponse))
					Expect(err).To(MatchError(customError))
				})
			})

		})
		Describe("CreateBlackBoxExporterDeployment", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
			})

			When("the resource(deployment) Exists", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
				})
				It("should call `Get` and not call `Create`", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterDeployment(ctx)
					//Assert
					Expect(err).NotTo(HaveOccurred())

				})
			})
			When("the resource(deployment) is Not Found", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = notFoundErr
					createCalledTimes = 1
				})
				It("should call `Get` successfully and `Create` the resource(deployment)", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterDeployment(ctx)
					//Assert
					Expect(err).NotTo(HaveOccurred())
				})
			})
			When("the resource(deployment) Get fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = customError
				})
				It("should return the error and not call `Create`", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterDeployment(ctx)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(getErrorResponse))
				})
			})
			When("the resource(deployment) Create fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = notFoundErr
					createCalledTimes = 1
					createErrorResponse = customError
				})
				It("should call `Get` Successfully and call `Create` but return the error", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterDeployment(ctx)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(createErrorResponse))
					Expect(err).To(MatchError(customError))
				})
			})
		})
		Describe("CreateBlackBoxExporterService", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
			})

			When("the resource(service) Exists", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
				})
				It("should call `Get` and not call `Create`", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterService(ctx)
					//Assert
					Expect(err).NotTo(HaveOccurred())

				})
			})
			When("the resource(service) is Not Found", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = notFoundErr
					createCalledTimes = 1
				})
				It("should call `Get` successfully and `Create` the resource(service)", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterService(ctx)
					//Assert
					Expect(err).NotTo(HaveOccurred())
				})
			})
			When("the resource(service) Get fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = customError
				})
				It("should return the error and not call `Create`", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterService(ctx)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(getErrorResponse))
				})
			})
			When("the resource(service) Create fails unexpectedly", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
					getErrorResponse = notFoundErr
					createCalledTimes = 1
					createErrorResponse = customError
				})
				It("should call `Get` Successfully and call `Create` but return the error", func() {
					//Act
					err := routeMonitorReconciler.CreateBlackBoxExporterService(ctx)
					//Assert
					Expect(err).To(HaveOccurred())
					Expect(err).To(MatchError(createErrorResponse))
					Expect(err).To(MatchError(customError))
				})
			})
		})
	})
})

func setScheme(scheme *runtime.Scheme) *runtime.Scheme {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(monitoringv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	return scheme
}
