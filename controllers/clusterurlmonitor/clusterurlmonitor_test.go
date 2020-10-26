package clusterurlmonitor_test

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	. "github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	"github.com/openshift/route-monitor-operator/pkg/consts/blackbox"
	constinit "github.com/openshift/route-monitor-operator/pkg/consts/test/init"
	"github.com/openshift/route-monitor-operator/pkg/util/reconcile"
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
		mockBlackboxExporter     *routemonitormocks.MockBlackboxExporter
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
		mockBlackboxExporter = routemonitormocks.NewMockBlackboxExporter(mockCtrl)
		clusterUrlMonitor = v1alpha1.ClusterUrlMonitor{}
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
			BlackboxExporter:  mockBlackboxExporter,
			Ctx:               context.TODO(),
			ClusterUrlMonitor: clusterUrlMonitor,
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
				)
				mockClient.EXPECT().Create(gomock.Any(), serviceMonitorMatcher).Times(1)
			})

			It("creates a ServiceMonitor with the service URL", func() {
				err := sup.EnsureServiceMonitorExists()
				expectedUrl := prefix + clusterDomain + ":" + port + suffix
				Expect(err).NotTo(HaveOccurred())
				Expect(serviceMonitorMatcher.Actual.Spec.Endpoints[0].Params["target"][0]).To(Equal(expectedUrl))
			})
		})
		When("the ServiceMonitor exists already", func() {
			BeforeEach(func() {
				serviceMonitor := monitoringv1.ServiceMonitor{}
				mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
					SetArg(2, serviceMonitor)
			})
			It("doesn't update the ServiceMonitor", func() {
				err := sup.EnsureServiceMonitorExists()
				Expect(err).NotTo(HaveOccurred())
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
						mockBlackboxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackbox.DeleteBlackBoxExporter, nil)
					})
					It("removes the servicemonitor, the blackbox exporter and cleans up the finalizer", func() {
						mockBlackboxExporter.EXPECT().EnsureBlackBoxExporterResourcesAbsent().Times(1)

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
						mockBlackboxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackbox.KeepBlackBoxExporter, nil)
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
						mockBlackboxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackbox.DeleteBlackBoxExporter, nil)
					})
					It("removes the blackbox exporter and cleans up the finalizer", func() {
						mockBlackboxExporter.EXPECT().EnsureBlackBoxExporterResourcesAbsent().Times(1)

						res, err := sup.EnsureDeletionProcessed()

						Expect(err).NotTo(HaveOccurred())
						Expect(res).To(Equal(reconcile.StopOperation()))
						Expect(clusterUrlMonitorMatcher.Actual.Finalizers).NotTo(ContainElement(FinalizerKey))
					})
				})

				When("the blackboxexporter doesn't need to be cleaned up", func() {
					BeforeEach(func() {
						mockBlackboxExporter.EXPECT().ShouldDeleteBlackBoxExporterResources().Return(blackbox.KeepBlackBoxExporter, nil)
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
