package clusterurlmonitor_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	customerrors "github.com/openshift/route-monitor-operator/pkg/util/errors"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	. "github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackboxexporter"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
	clientmocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/client"
	routemonitormocks "github.com/openshift/route-monitor-operator/pkg/util/test/generated/mocks/routemonitor"
)

type ClusterUrlMonitorMatcher struct {
	Actual     v1alpha1.ClusterUrlMonitor
	FailReason string
}

func (m *ClusterUrlMonitorMatcher) Matches(x interface{}) bool {
	ref, isCorrectType := x.(*v1alpha1.ClusterUrlMonitor)
	if !isCorrectType {
		m.FailReason = fmt.Sprintf("Unexpected type passed: want '%T', got '%T'", v1alpha1.ClusterUrlMonitor{}, x)
		return false
	}
	m.Actual = *ref.DeepCopy()
	return true
}

func (m *ClusterUrlMonitorMatcher) String() string {
	return "Fail reason: " + m.FailReason
}

type ServiceMonitorMatcher struct {
	Actual     monitoringv1.ServiceMonitor
	FailReason string
}

func (m *ServiceMonitorMatcher) Matches(x interface{}) bool {
	ref, isCorrectType := x.(*monitoringv1.ServiceMonitor)
	if !isCorrectType {
		m.FailReason = fmt.Sprintf("Unexpected type passed: want '%T', got '%T'", monitoringv1.ServiceMonitor{}, x)
		return false
	}
	m.Actual = *ref.DeepCopy()
	return true
}

func (m *ServiceMonitorMatcher) String() string {
	return "Fail reason: " + m.FailReason
}

var _ = Describe("Clusterurlmonitor", func() {
	var (
		clusterUrlMonitor        v1alpha1.ClusterUrlMonitor
		sup                      ClusterUrlMonitorSupplement
		mockClient               *clientmocks.MockClient
		mockBlackBoxExporter     *routemonitormocks.MockBlackBoxExporter
		mockResourceComparer     *routemonitormocks.MockResourceComparer
		mockStatusWriter         *clientmocks.MockStatusWriter
		mockCtrl                 *gomock.Controller
		clusterUrlMonitorMatcher *ClusterUrlMonitorMatcher
		serviceMonitorMatcher    *ServiceMonitorMatcher

		fakeNotFound error

		prefix string
		port   string
		suffix string
	)

	BeforeEach(func() {
		mockCtrl = gomock.NewController(GinkgoT())
		mockClient = clientmocks.NewMockClient(mockCtrl)
		mockBlackBoxExporter = routemonitormocks.NewMockBlackBoxExporter(mockCtrl)
		mockResourceComparer = routemonitormocks.NewMockResourceComparer(mockCtrl)
		mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)
		clusterUrlMonitor = v1alpha1.ClusterUrlMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake-clusterurlmonitor",
				Namespace: "fake-namespace",
			},
		}
		clusterUrlMonitorMatcher = &ClusterUrlMonitorMatcher{}
		serviceMonitorMatcher = &ServiceMonitorMatcher{}
		fakeNotFound = k8serrors.NewNotFound(schema.GroupResource{}, "fake-error")
	})

	JustBeforeEach(func() {
		clusterUrlMonitor.Spec.Prefix = prefix
		clusterUrlMonitor.Spec.Suffix = suffix
		clusterUrlMonitor.Spec.Port = port
		sup = ClusterUrlMonitorSupplement{
			Log:               constinit.Logger,
			Client:            mockClient,
			BlackBoxExporter:  mockBlackBoxExporter,
			Ctx:               context.TODO(),
			ClusterUrlMonitor: clusterUrlMonitor,
			ResourceComparer:  mockResourceComparer,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Describe("EnsureFinalizer", func() {
		When("the finalizer doesn't exists", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Update(gomock.Any(), clusterUrlMonitorMatcher).Times(1)
			})
			It("adds the finalizer to the CR", func() {
				res, err := sup.EnsureFinalizer()
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(reconcile.StopOperation()))
				Expect(clusterUrlMonitorMatcher.Actual.Finalizers).To(ContainElement(FinalizerKey))
			})
		})

		When("the finalizer already exists", func() {
			BeforeEach(func() {
				clusterUrlMonitor.Finalizers = []string{clusterurlmonitor.FinalizerKey}
			})
			It("doesn't update the ClusterUrlMonitor", func() {
				mockClient.EXPECT().Update(gomock.Any(), clusterUrlMonitorMatcher).Times(0)
				res, err := sup.EnsureFinalizer()
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(reconcile.ContinueOperation()))
			})
		})
	})

	Describe("EnsureServiceMonitorExists", func() {
		var (
			clusterDomain string
		)
		BeforeEach(func() {
			clusterDomain = "fake.test"
			port = "1337"
			prefix = "prefix."
			suffix = "/suffix"
		})
		When("the ServiceMonitor doesn't exist", func() {
			BeforeEach(func() {
				ingress := configv1.Ingress{
					Spec: configv1.IngressSpec{
						Domain: clusterDomain,
					},
				}
				gomock.InOrder(
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeNotFound),
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, ingress),
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeNotFound),
				)
				mockClient.EXPECT().Create(gomock.Any(), serviceMonitorMatcher).Times(1)
				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				mockStatusWriter.EXPECT().Update(gomock.Any(), clusterUrlMonitorMatcher).Times(1).Return(nil)
				mockBlackBoxExporter.EXPECT().GetBlackBoxExporterNamespace().Times(1).Return("")
			})

			It("creates a ServiceMonitor with the service URL", func() {
				_, err := sup.EnsureServiceMonitorExists()
				expectedUrl := prefix + clusterDomain + ":" + port + suffix
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceMonitorMatcher.Actual.Spec.Endpoints[0].Params["target"][0]).To(Equal(expectedUrl))
				Expect(clusterUrlMonitorMatcher.Actual.Status.ServiceMonitorRef.Name).To(Equal(serviceMonitorMatcher.Actual.Name))
				Expect(clusterUrlMonitorMatcher.Actual.Status.ServiceMonitorRef.Namespace).To(Equal(serviceMonitorMatcher.Actual.Namespace))
			})
		})
		When("the ServiceMonitor exists already", func() {
			BeforeEach(func() {
				serviceMonitor := monitoringv1.ServiceMonitor{}
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
					SetArg(2, serviceMonitor)
			})

			It("doesn't update the ServiceMonitor", func() {
				_, err := sup.EnsureServiceMonitorExists()
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("EnsurePrometheusRuleResourceExists", func() {
		var (
			mockStatusWriter     *clientmocks.MockStatusWriter
			errorToPopulate      error
			shouldPopulateError  bool
			deepEqualCalledTimes int
			deepEqualResponse    bool
		)
		BeforeEach(func() {
			// Why bellow code?
			// routeMonitorSlo = v1alpha1.SloSpec{}
			// clusterUrlMonitorSlo = v1alpha1.SloSpec{
			// 	TargetAvailabilityPercent: "99.95",
			// }
			// clusterUrlMonitor.Spec = v1alpha1.clusterUrlMonitor{
			// 	Slo: clusterUrlMonitorSlo,
			// }

			mockStatusWriter = clientmocks.NewMockStatusWriter(mockCtrl)
			errorToPopulate = nil
			shouldPopulateError = false
			deepEqualCalledTimes = 0
			deepEqualResponse = true
			port = "1337"
			prefix = "prefix."
			suffix = "/suffix"

			clusterUrlMonitor.Name = "fake-clusterurlmonitor"
			clusterUrlMonitor.Namespace = "fake-namespace"
			clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "99.95"
		})
		JustBeforeEach(func() {
			mockResourceComparer.EXPECT().DeepEqual(gomock.Any(), gomock.Any()).Return(deepEqualResponse).Times(deepEqualCalledTimes)

			expectedClusterUrlMonitor := clusterUrlMonitor

			if shouldPopulateError {
				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				expectedClusterUrlMonitor.Status.ErrorStatus = errorToPopulate.Error()
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Eq(&expectedClusterUrlMonitor)).
					Times(1).
					Return(nil)
			}
		})
		When("the ClusterUrlMonitor has an empty slo value", func() {
			// Arrange
			BeforeEach(func() {
				clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = ""
			})
			It("should skip processing and continue", func() {
				// Act
				res, err := sup.EnsurePrometheusRuleResourceExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(res).NotTo(BeNil())
				Expect(res).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
		When("the ClusterUrlMonitor has a slo spec but percent is too low", func() {
			// Arrange
			BeforeEach(func() {
				clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "-0/10"
				shouldPopulateError, errorToPopulate = true, customerrors.InvalidSLO
			})
			It("should Throw an error", func() {
				// Act
				_, err := sup.EnsurePrometheusRuleResourceExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the ClusterUrlMonitor has a slo spec but percent is too high", func() {
			// Arrange
			BeforeEach(func() {
				clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "101"
				shouldPopulateError, errorToPopulate = true, customerrors.InvalidSLO
			})
			It("should Throw an error", func() {
				// Act
				_, err := sup.EnsurePrometheusRuleResourceExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("the ClusterUrlMonitor has an invalid slo type", func() {
			// Arrange
			BeforeEach(func() {
				clusterUrlMonitor.Spec.Slo.TargetAvailabilityPercent = "fake-slo-type"
				shouldPopulateError, errorToPopulate = true, customerrors.InvalidSLO
			})
			It("should Throw an error", func() {
				// Act
				_, err := sup.EnsurePrometheusRuleResourceExists()
				// Assert
				Expect(err).NotTo(HaveOccurred())
			})
		})
		Describe("the PrometheusRuleResource exists but...", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)
				deepEqualCalledTimes = 1
			})
			JustBeforeEach(func() {
				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil)
				//Act
				resp, err := sup.EnsurePrometheusRuleResourceExists()
				//Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.StopOperation()))
			})
			When("deepEqual is true (they are the same)", func() {
				It("should stop operation", func() {})
			})
			When("deepEqual is false (they are different)", func() {
				BeforeEach(func() {
					deepEqualResponse = false
					mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1)
				})
				It("should stop operation", func() {})
			})
		})
		When("the resource Exists but not the same as the generated template", func() {
			BeforeEach(func() {
				mockClient.EXPECT().Status().Return(mockStatusWriter).Times(1)
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)

				deepEqualResponse = false
				deepEqualCalledTimes = 1

				mockClient.EXPECT().Update(gomock.Any(), gomock.Any()).Times(1)
			})

			JustBeforeEach(func() {
				mockStatusWriter.EXPECT().Update(gomock.Any(), gomock.Any()).
					Times(1).
					Return(nil)
			})
			It("should call `Get` and `Update` and not call `Create` and stop reconciling", func() {
				//Act
				resp, err := sup.EnsurePrometheusRuleResourceExists()
				//Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.StopOperation()))
			})
		})
		When("the EnsurePrometheusRuleResourceExists should pass all checks", func() {
			BeforeEach(func() {
				ingress := configv1.Ingress{
					Spec: configv1.IngressSpec{
						Domain: "fake.test",
					},
				}
				clusterUrlMonitor.Status.PrometheusRuleRef = v1alpha1.NamespacedName{
					Namespace: clusterUrlMonitor.Namespace,
					Name:      clusterUrlMonitor.Name,
				}

				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, ingress)
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

				deepEqualResponse = true
				deepEqualCalledTimes = 1
			})

			It("should continue reconciling", func() {
				//Act
				resp, err := sup.EnsurePrometheusRuleResourceExists()
				//Assert
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).NotTo(BeNil())
				Expect(resp).To(Equal(utilreconcile.ContinueOperation()))
			})
		})
	})

	Describe("EnsureDeletionProcessed", func() {
		BeforeEach(func() {
			clusterUrlMonitor.Finalizers = []string{FinalizerKey}
		})
		When("the ClusterUrlMonitor CR is not being deleted", func() {
			It("does nothing", func() {
				res, err := sup.EnsureDeletionProcessed()
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(reconcile.ContinueOperation()))
			})
		})

		When("the ClusterUrlMonitor CR is being deleted", func() {
			BeforeEach(func() {
				clusterUrlMonitor.DeletionTimestamp = &metav1.Time{Time: time.Unix(0, 0)}
				mockClient.EXPECT().Update(gomock.Any(), clusterUrlMonitorMatcher)
			})
			When("the ServiceMonitor still exists", func() {
				var (
					serviceMonitor monitoringv1.ServiceMonitor
				)
				BeforeEach(func() {
					serviceMonitor = monitoringv1.ServiceMonitor{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "fake-name",
							Namespace: "fake-namespace",
						},
					}
					mockClient.EXPECT().Delete(gomock.Any(), serviceMonitorMatcher)
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).SetArg(2, serviceMonitor)
				})
				When("the blackboxexporter needs to be cleaned up", func() {
					BeforeEach(func() {
						mockBlackBoxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackboxexporter.DeleteBlackBoxExporter, nil)
					})
					It("removes the servicemonitor, the blackbox exporter and cleans up the finalizer", func() {
						mockBlackBoxExporter.EXPECT().EnsureBlackBoxExporterResourcesAbsent().Times(1)

						res, err := sup.EnsureDeletionProcessed()

						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(reconcile.StopOperation()))
						Expect(serviceMonitorMatcher.Actual.Name).To(Equal(serviceMonitor.Name))
						Expect(serviceMonitorMatcher.Actual.Namespace).To(Equal(serviceMonitor.Namespace))
						Expect(clusterUrlMonitorMatcher.Actual.Finalizers).NotTo(ContainElement(FinalizerKey))
					})
				})

				When("the blackboxexporter doesn't need to be cleaned up", func() {
					BeforeEach(func() {
						mockBlackBoxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackboxexporter.KeepBlackBoxExporter, nil)
					})
					It("removes the servicemonitor and cleans up the finalizer", func() {
						res, err := sup.EnsureDeletionProcessed()

						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(reconcile.StopOperation()))
						Expect(serviceMonitorMatcher.Actual.Name).To(Equal(serviceMonitor.Name))
						Expect(serviceMonitorMatcher.Actual.Namespace).To(Equal(serviceMonitor.Namespace))
					})
				})
			})

			When("the ServiceMonitor doesn't exist", func() {
				BeforeEach(func() {
					mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fakeNotFound)
				})
				When("the blackboxexporter needs to be cleaned up", func() {
					BeforeEach(func() {
						mockBlackBoxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackboxexporter.DeleteBlackBoxExporter, nil)
					})
					It("removes the blackbox exporter and cleans up the finalizer", func() {
						mockBlackBoxExporter.EXPECT().EnsureBlackBoxExporterResourcesAbsent().Times(1)

						res, err := sup.EnsureDeletionProcessed()

						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(reconcile.StopOperation()))
						Expect(clusterUrlMonitorMatcher.Actual.Finalizers).NotTo(ContainElement(FinalizerKey))
					})
				})

				When("the blackboxexporter doesn't need to be cleaned up", func() {
					BeforeEach(func() {
						mockBlackBoxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackboxexporter.KeepBlackBoxExporter, nil)
					})
					It("cleans up the finalizer", func() {
						res, err := sup.EnsureDeletionProcessed()

						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(reconcile.StopOperation()))
						Expect(clusterUrlMonitorMatcher.Actual.Finalizers).NotTo(ContainElement(FinalizerKey))
					})
				})
			})

		})
	})
})
