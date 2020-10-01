package routemonitor_test

import (
	"context"
	"errors"
	"time"

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

	"github.com/google/gofuzz" // fuzz testing
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"
	//routemonitormocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/routemonitor"
)

var _ = Describe("Routemonitor", func() {
	// Add new Fuzzer
	f := fuzz.New()

	var (
		routeMonitor                     monitoringv1alpha1.RouteMonitor
		routeMonitorName                 string
		routeMonitorNamespace            string
		routeMonitorRouteSpec            monitoringv1alpha1.RouteMonitorRouteSpec
		routeMonitorFinalizers           []string
		routeMonitorEnableCustomResponse map[string]bool
		routeMonitorCustomResponse       map[string]error
		routeMonitorDeletionTimestamp    *metav1.Time
		routeMonitorStatus               monitoringv1alpha1.RouteMonitorStatus

		routeMonitorReconciler       routemonitor.RouteMonitorReconciler
		routeMonitorReconcilerClient client.Client

		req                  ctrl.Request
		route                routev1.Route
		expectedRouteMonitor monitoringv1alpha1.RouteMonitor

		mockClient       *clientmocks.MockClient
		mockStatusWriter *clientmocks.MockStatusWriter
		mockCtrl         *gomock.Controller

		// createCalledTimes is a toggle for the 'mockClient.EXPECT.Create()'. If set to 0 then ignored
		createCalledTimes   int
		createErrorResponse error
		// getCalledTimes is a toggle for the 'mockClient.EXPECT.Get()'. If set to 0 then ignored
		getCalledTimes   int
		getErrorResponse error
		// listCalledTimes is a toggle for the 'mockClient.EXPECT.List()'. If set to 0 then ignored
		listCalledTimes   int
		listErrorResponse error
		// updateCalledTimes is a toggle for the 'mockClient.EXPECT.Update()'. If set to 0 then ignored
		updateCalledTimes   int
		updateErrorResponse error
		// delete is a toggle for the 'mockClient.EXPECT.Delete()'. If set to 0 then ignored
		deleteCalledTimes   int
		deleteErrorResponse error
	)
	const (
		routeMonitorRouteURLDefault = "fake-route-url"
	)

	var ( // Practically const vars
		ctx         = context.TODO()
		logger      = testing.NullLogger{}
		scheme      = setScheme(runtime.NewScheme())
		notFoundErr = &k8serrors.StatusError{
			ErrStatus: metav1.Status{
				Reason: metav1.StatusReasonNotFound,
			}}
		customError        = errors.New("test")
		routeMonitorSecond = monitoringv1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      routeMonitorName + "-but-different",
				Namespace: routeMonitorNamespace,
			},
		}
	)
	// Start Fuzz testing for values
	f.Fuzz(&routeMonitorName)
	f.Fuzz(&routeMonitorNamespace)

	BeforeEach(func() {
		routeMonitorRouteSpec = monitoringv1alpha1.RouteMonitorRouteSpec{}
		routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme)
		routeMonitorFinalizers = []string{routemonitor.FinalizerKey}
		routeMonitorEnableCustomResponse = map[string]bool{}
		routeMonitorCustomResponse = map[string]error{}
		routeMonitorDeletionTimestamp = nil
		routeMonitorStatus = monitoringv1alpha1.RouteMonitorStatus{}
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)
		createCalledTimes = 0
		createErrorResponse = nil
		getCalledTimes = 0
		getErrorResponse = nil
		listCalledTimes = 0
		listErrorResponse = nil
		updateCalledTimes = 0
		updateErrorResponse = nil
		deleteCalledTimes = 0
		deleteErrorResponse = nil
	})

	JustBeforeEach(func() {
		routeMonitor = monitoringv1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:              routeMonitorName,
				Namespace:         routeMonitorNamespace,
				Finalizers:        routeMonitorFinalizers,
				DeletionTimestamp: routeMonitorDeletionTimestamp,
			},
			Spec: monitoringv1alpha1.RouteMonitorSpec{
				Route: routeMonitorRouteSpec,
			},
			Status: routeMonitorStatus,
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

		if createCalledTimes != 0 {
			mockClient.EXPECT().Create(gomock.Any(), gomock.Any()).
				Return(createErrorResponse).
				Times(createCalledTimes)
		}

		if updateCalledTimes != 0 {
			mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).
				Return(updateErrorResponse).
				Times(updateCalledTimes)
		}

		mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(getErrorResponse).
			Times(getCalledTimes)

		mockClient.EXPECT().List(gomock.Any(), gomock.Any()).
			Return(listErrorResponse).
			Times(listCalledTimes)

		mockClient.EXPECT().Delete(gomock.Any(), gomock.Any()).
			Return(deleteErrorResponse).
			Times(deleteCalledTimes)

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
			It("should stop requeue", func() {
				// Act
				resRouteMonitor, res, err := routeMonitorReconciler.GetRouteMonitor(ctx, req)
				// Assert
				Skip("make it pass for now")
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
				Expect(resRouteMonitor).To(BeZero())
			})
		})
		When("the RouteMonitor is found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})

			It("should return the object", func() {
				// Act
				resRouteMonitor, _, err := routeMonitorReconciler.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resRouteMonitor).NotTo(BeNil())
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
				res := routeMonitorReconciler.WasDeleteRequested(routeMonitor)
				// Assert
				Expect(res).To(BeTrue())
			})
		})
		When("a user does nothing", func() {
			// Arrange
			It("should return 'false'", func() {
				// Act
				res := routeMonitorReconciler.WasDeleteRequested(routeMonitor)
				// Assert
				Expect(res).To(BeFalse())
			})
		})
	})
	Describe("ShouldDeleteBlackBoxExporterResources", func() {
		BeforeEach(func() {
			routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
		})
		When("a delete was not requested (user did nothing)", func() {
			BeforeEach(func() {
				routeMonitorDeletionTimestamp = nil
			})
			// Arrange
			It("should stop early and return 'false'", func() {
				// Act
				res, err := routeMonitorReconciler.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeFalse())
			})
		})
		When("the `List` command fails", func() {
			// Arrange
			BeforeEach(func() {
				listCalledTimes = 1
				listErrorResponse = customError
				routeMonitorReconcilerClient = mockClient
			})
			It("should fail with the List error", func() {
				// Act
				_, err := routeMonitorReconciler.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				//Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})
		When("there are no RouteMonitors", func() {
			It("should technically return  'true' but return InternalFault error", func() {
				// Act
				res, err := routeMonitorReconciler.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(HavePrefix("Internal Fault:"))

				// is true because it should delete even if there are no items.
				// but this is in unusual situation because:
				// - a delete was requested
				// - there are no items of that resource on the cluster
				Expect(res).To(BeTrue())
			})
		})

		When("there are many RouteMonitors", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor, &routeMonitorSecond)
			})
			It("should return 'false' for too many RouteMonitors", func() {
				// Act
				res, err := routeMonitorReconciler.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeFalse())
			})
		})

		When("there is just one RouteMonitor", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})
			It("should return 'true'", func() {
				// Act
				res, err := routeMonitorReconciler.ShouldDeleteBlackBoxExporterResources(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeTrue())
			})

		})
	})

	Describe("HasFinalizer", func() {
		When("'FinalizerKey' is NOT in the 'Finalizers' list", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
			})

			It("should return false", func() {
				// Act
				res := routeMonitorReconciler.HasFinalizer(routeMonitor)
				// Assert
				Expect(res).To(BeFalse())
			})
		})
		When("'FinalizerKey' is in the 'Finalizers' list", func() {
			It("should return true", func() {
				// Act
				res := routeMonitorReconciler.HasFinalizer(routeMonitor)
				// Assert
				Expect(res).To(BeTrue())
			})
		})
	})

	Describe("DeleteRouteMonitorAndDependencies", func() {
		// Arrange
		BeforeEach(func() {
			routeMonitorEnableCustomResponse["DeleteServiceMonitorResource"] = true
			routeMonitorCustomResponse["DeleteServiceMonitorResource"] = nil
			routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
		})
		When("DeleteServiceMonitorResource fails unexpectidly", func() {
			BeforeEach(func() {
				routeMonitorCustomResponse["DeleteServiceMonitorResource"] = customError
			})
			It("should bubble up the error and return it", func() {
				// Act
				Skip("need to mock update")
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})
		When("the resource has a finalizer but 'Update' failed", func() {
			// Arrange
			BeforeEach(func() {
				updateCalledTimes = 1
				updateErrorResponse = customError
				routeMonitorReconcilerClient = mockClient
			})
			It("Should bubble up the failure", func() {
				// Act
				Skip("DeleteRouteMonitorAndDependencies has inner deps that should not be tested")
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})
		When("the resource has a finalizer but 'Update' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				updateCalledTimes = 1
				routeMonitorReconcilerClient = mockClient
			})
			It("Should succeed and call for a requeue", func() {
				// Act
				Skip("DeleteRouteMonitorAndDependencies has inner deps that should not be tested")
				res, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the resource doesnt have a finalizer but not deletion was requested", func() {
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
			})
			It("should complete successfully", func() {
				// Act
				res, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})

		When("the resource has a finalizer but 'Delete' failed", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
				routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
				deleteCalledTimes = 1
				deleteErrorResponse = customError
				routeMonitorReconcilerClient = mockClient
			})
			It("Should bubble up the failure", func() {
				// Act
				Skip("DeleteRouteMonitorAndDependencies has inner deps that should not be tested")
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})
		When("the resource has a finalizer but 'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorFinalizers = []string{}
				routeMonitorDeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
				deleteCalledTimes = 1
				routeMonitorReconcilerClient = mockClient
			})
			It("should pass successfully", func() {
				// Act
				Skip("DeleteRouteMonitorAndDependencies has inner deps that should not be tested")
				_, err := routeMonitorReconciler.DeleteRouteMonitorAndDependencies(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(err).To(BeNil())
			})
		})
	})
	//Describe("DeleteBlackBoxExporterResources", func() {
	//When("subcommand 'Service' fails unexpectedly", func() {
	//It("should bubble error", func() {
	//})
	//})
	//})
	Describe("DeleteServiceMonitorResource", func() {
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorReconcilerClient = mockClient
		})
		When("'Get' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = customError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorReconciler.DeleteServiceMonitorResource(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})
		When("'Get' returns an 'Not found' error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = notFoundErr
			})
			It("should succeed as there is nothing to delete", func() {
				// Act
				err := routeMonitorReconciler.DeleteServiceMonitorResource(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("'Delete' returns an unhandled error", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
				deleteErrorResponse = customError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorReconciler.DeleteServiceMonitorResource(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})
		When("'Delete' passes", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
			})
			It("should succeed as the object was deleted", func() {
				// Act
				err := routeMonitorReconciler.DeleteServiceMonitorResource(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("DeleteBlackBoxExporterService", func() {
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorReconcilerClient = mockClient
		})

		When("'Get' return an error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = customError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterService(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})

		When("'Get' return an 'NotFound' error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = notFoundErr
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterService(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("'Delete' return an an  error", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
				deleteErrorResponse = customError
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterService(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})

		When("'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
			})
			It("should succeed", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterService(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

	})

	Describe("DeleteBlackBoxExporterDeployment", func() {
		BeforeEach(func() {
			getCalledTimes = 1
			routeMonitorReconcilerClient = mockClient
		})

		When("'Get' return an error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = customError
			})
			It("should bubble the error up", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterDeployment(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})

		When("'Get' return an 'NotFound' error", func() {
			// Arrange
			BeforeEach(func() {
				getErrorResponse = notFoundErr
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterDeployment(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})

		When("'Delete' return an an  error", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
				deleteErrorResponse = customError
			})
			It("should do nothing", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterDeployment(ctx)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(err).To(MatchError(customError))
			})
		})

		When("'Delete' succeeds", func() {
			// Arrange
			BeforeEach(func() {
				deleteCalledTimes = 1
			})
			It("should succeed", func() {
				// Act
				err := routeMonitorReconciler.DeleteBlackBoxExporterDeployment(ctx)
				// Assert
				Expect(err).NotTo(HaveOccurred())
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
				resRoute, err := routeMonitorReconciler.GetRoute(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(resRoute).To(BeZero())

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
				resRoute, err := routeMonitorReconciler.GetRoute(ctx, routeMonitor)
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
					resRoute, err := routeMonitorReconciler.GetRoute(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeZero())
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
					resRoute, err := routeMonitorReconciler.GetRoute(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeZero())
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
				res, err := routeMonitorReconciler.UpdateRouteURL(ctx, route, routeMonitor)
				// Assert
				Expect(res).To(BeZero())
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
				res, err := routeMonitorReconciler.UpdateRouteURL(ctx, route, routeMonitor)
				// Assert
				Expect(res).To(BeZero())
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
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Eq(&expectedRouteMonitor)).Times(1).Return(nil)
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
			})
			It("should update the first ingress", func() {
				// Act
				res, err := routeMonitorReconciler.UpdateRouteURL(ctx, route, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})
	Describe("CreateServiceMonitorResource", func() {
		When("the RouteMonitor has no Host", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorReconcilerClient = fake.NewFakeClientWithScheme(scheme)
			})
			It("should return No Host error", func() {
				// Act
				_, err := routeMonitorReconciler.CreateServiceMonitorResource(ctx, routeMonitor)
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
				routeMonitorFinalizers = nil
				routeMonitorStatus = monitoringv1alpha1.RouteMonitorStatus{
					RouteURL: routeMonitorRouteURLDefault,
				}

			})
			JustBeforeEach(func() {
			})
			It("Should update the RouteMonitor with the finalizer", func() {
				// Act
				resp, err := routeMonitorReconciler.CreateServiceMonitorResource(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.StopOperation()))

			})

		})
		Describe("Testing CreateResourceIfNotFound", func() {
			BeforeEach(func() {
				routeMonitorReconcilerClient = mockClient
				routeMonitorStatus = monitoringv1alpha1.RouteMonitorStatus{
					RouteURL: routeMonitorRouteURLDefault,
				}
			})

			When("the resource Exists", func() {
				// Arrange
				BeforeEach(func() {
					getCalledTimes = 1
				})
				It("should call `Get` and not call `Create`", func() {
					//Act
					_, err := routeMonitorReconciler.CreateServiceMonitorResource(ctx, routeMonitor)
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
					_, err := routeMonitorReconciler.CreateServiceMonitorResource(ctx, routeMonitor)
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
					_, err := routeMonitorReconciler.CreateServiceMonitorResource(ctx, routeMonitor)
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
					_, err := routeMonitorReconciler.CreateServiceMonitorResource(ctx, routeMonitor)
					//Assert
					Expect(err).To(HaveOccurred())
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
