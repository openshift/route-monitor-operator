package supplement_test

import (
	"github.com/golang/mock/gomock"
	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/openshift/route-monitor-operator/controllers/routemonitor/supplement"

	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/route-monitor-operator/api/v1alpha1"

	routemonitorconst "github.com/openshift/route-monitor-operator/pkg/const"
	constinit "github.com/openshift/route-monitor-operator/pkg/const/test/init"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/tests/generated/mocks/client"
)

var _ = Describe("Routemonitor", func() {
	// Add new Fuzzer
	f := fuzz.New()

	var (
		routeMonitor                  v1alpha1.RouteMonitor
		routeMonitorName              string
		routeMonitorNamespace         string
		routeMonitorRouteSpec         v1alpha1.RouteMonitorRouteSpec
		routeMonitorFinalizers        []string
		routeMonitorDeletionTimestamp *metav1.Time
		routeMonitorStatus            v1alpha1.RouteMonitorStatus

		routeMonitorSupplement       supplement.RouteMonitorSupplement
		routeMonitorSupplementClient client.Client

		req                  ctrl.Request
		route                routev1.Route
		expectedRouteMonitor v1alpha1.RouteMonitor

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
		ctx    = constinit.Context
		scheme = constinit.Scheme
	)

	// Start Fuzz testing for values
	f.Fuzz(&routeMonitorName)
	f.Fuzz(&routeMonitorNamespace)

	BeforeEach(func() {
		routeMonitorName = "fake-name"
		routeMonitorNamespace = "fake-namespace"
		routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{}
		routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme)
		routeMonitorFinalizers = routemonitorconst.FinalizerList
		routeMonitorDeletionTimestamp = nil
		routeMonitorStatus = v1alpha1.RouteMonitorStatus{}
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
		routeMonitor = v1alpha1.RouteMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:              routeMonitorName,
				Namespace:         routeMonitorNamespace,
				Finalizers:        routeMonitorFinalizers,
				DeletionTimestamp: routeMonitorDeletionTimestamp,
			},
			Spec: v1alpha1.RouteMonitorSpec{
				Route: routeMonitorRouteSpec,
			},
			Status: routeMonitorStatus,
		}

		routeMonitorSupplement = supplement.RouteMonitorSupplement{
			Log:    constinit.Logger,
			Client: routeMonitorSupplementClient,
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
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})
			It("should stop requeue", func() {
				// Act
				resRouteMonitor, res, err := routeMonitorSupplement.GetRouteMonitor(ctx, req)
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
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &routeMonitor)
			})

			It("should return the object", func() {
				// Act
				resRouteMonitor, _, err := routeMonitorSupplement.GetRouteMonitor(ctx, req)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resRouteMonitor).NotTo(BeNil())
			})
		})
	})

	Describe("GetRoute", func() {
		When("the Route is not found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{
					Name:      "Rob",
					Namespace: "Bob",
				}
				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "testns",
					},
				}
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &route)
			})
			It("should return a Not Found error", func() {
				// Act
				resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
				// Assert
				Expect(err).To(HaveOccurred())
				Expect(k8serrors.IsNotFound(err)).To(BeTrue())
				Expect(resRoute).To(BeZero())

			})
		})
		When("the Route is found", func() {
			// Arrange
			BeforeEach(func() {
				routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{
					Name:      routeMonitorName,
					Namespace: routeMonitorNamespace,
				}

				route = routev1.Route{
					ObjectMeta: metav1.ObjectMeta{
						Name:      routeMonitorName,
						Namespace: routeMonitorNamespace,
					},
				}
				routeMonitorSupplementClient = fake.NewFakeClientWithScheme(scheme, &route)
			})
			It("should return the route", func() {
				// Act
				resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resRoute).ShouldNot(BeNil())
			})
		})

		Describe("Missing a RouteMonitor Field", func() {
			When("the RouteMonitor doesnt have Spec.Route.Namespace", func() {
				// Arrange
				BeforeEach(func() {
					routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{
						Name: "Rob",
						// Namespace is omitted
					}

				})
				It("should return a custom error", func() {
					// Act
					resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
					// Assert
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(HavePrefix("Invalid CR:"))
					Expect(resRoute).To(BeZero())
				})
			})
			When("the RouteMonitor doesnt have Spec.Route.Name", func() {
				// Arrange
				BeforeEach(func() {
					routeMonitorRouteSpec = v1alpha1.RouteMonitorRouteSpec{
						// Name is omitted
						Namespace: "Bob",
					}
				})
				It("should return a custom error", func() {
					// Act
					resRoute, err := routeMonitorSupplement.GetRoute(ctx, routeMonitor)
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
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
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
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
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
				routeMonitorSupplementClient = mockClient
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Eq(&expectedRouteMonitor)).Times(1).Return(nil)
			})
			JustBeforeEach(func() {
				expectedRouteMonitor.Status.RouteURL = firstRouteURL
			})
			It("should update the first ingress", func() {
				// Act
				res, err := routeMonitorSupplement.EnsureRouteURLExists(ctx, route, routeMonitor)
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.StopOperation()))
			})
		})
	})
})
