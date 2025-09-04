/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	avov1alpha2 "github.com/openshift/aws-vpce-operator/api/v1alpha2"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	rmov1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/config"
	"github.com/openshift/route-monitor-operator/controllers/clusterurlmonitor"
	"github.com/openshift/route-monitor-operator/controllers/hostedcontrolplane"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	rhobsv1 "github.com/rhobs/obo-prometheus-operator/pkg/apis/monitoring/v1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rmov1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(operatorv1.AddToScheme(scheme))
	utilruntime.Must(rmov1alpha1.AddToScheme(scheme))
	utilruntime.Must(hypershiftv1beta1.AddToScheme(scheme))
	utilruntime.Must(rhobsv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(avov1alpha2.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var enablehypershift bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enablehypershift, "enable-hypershift", false,
		"Enabling this for HyperShift")

	var blackboxExporterImage string
	var blackboxExporterNamespace string
	var probeAPIURL string

	flag.StringVar(&blackboxExporterImage, "blackbox-image", "quay.io/prometheus/blackbox-exporter@sha256:b04a9fef4fa086a02fc7fcd8dcdbc4b7b35cc30cdee860fdc6a19dd8b208d63e", "The image that will be used for the blackbox-exporter deployment")
	flag.StringVar(&blackboxExporterNamespace, "blackbox-namespace", config.OperatorNamespace, "Blackbox-exporter deployment will reside on this Namespace")
	flag.StringVar(&probeAPIURL, "probe-api-url", "", "The fully qualified API URL for RHOBS synthetics probe management (for HostedCluster monitoring). When empty, uses default blackbox exporter behavior.")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// Validate probe API URL format (if provided)
	if probeAPIURL != "" && !strings.HasPrefix(probeAPIURL, "http://") && !strings.HasPrefix(probeAPIURL, "https://") {
		setupLog.Error(nil, "probe-api-url must be a fully qualified URL starting with 'http://' or 'https://'", "probeAPIURL", probeAPIURL)
		os.Exit(1)
	}

	enableHCP, err := shouldEnableHCP()
	if err != nil {
		setupLog.Error(err, "failed to determine whether HCP controller should be enabled", "controller", "HostedControlPlane")
	}

	cacheOptions := cache.Options{}

	// If HCP is not enabled (RMO is not running on an MC cluster) then limit caching
	if !enableHCP {
		cacheOptions = cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				config.OperatorNamespace: {},
			},
			ByObject: map[client.Object]cache.ByObject{
				&rmov1alpha1.RouteMonitor{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
				&rmov1alpha1.ClusterUrlMonitor{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
				&routev1.Route{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
				&monitoringv1.ServiceMonitor{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
				&monitoringv1.PrometheusRule{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
				&operatorv1.IngressController{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
				&corev1.Service{}: {
					Namespaces: map[string]cache.Config{
						cache.AllNamespaces: {},
					},
				},
			},
		}
	}

	options := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "2793210b.openshift.io",
		Cache:                  cacheOptions,
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	routeMonitorReconciler := routemonitor.NewReconciler(mgr, blackboxExporterImage, blackboxExporterNamespace, enablehypershift, probeAPIURL)
	if err := routeMonitorReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RouteMonitor")
		os.Exit(1)
	}

	clusterUrlMonitorReconciler := clusterurlmonitor.NewReconciler(mgr, blackboxExporterImage, blackboxExporterNamespace, enablehypershift, probeAPIURL)
	if err := clusterUrlMonitorReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "clusterUrlMonitorReconciler")
		os.Exit(1)
	}

	if enableHCP {
		hostedControlPlaneReconciler := hostedcontrolplane.NewHostedControlPlaneReconciler(mgr, probeAPIURL)
		if err = hostedControlPlaneReconciler.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "HostedControlPlane")
			os.Exit(1)
		}
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.V(2).Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	setupLog.V(1).Info("`mgr.Start` is blocking:",
		"so this message (or anything that resides after it) won't execute until teardown", nil)
}

// shouldEnableHCP checks for the existence of the 'hostedcontrolplane' CRD to determine whether this controller should be enabled or not:
//   - if it exists, enable the HCP controller
//   - if we get an error unrelated to it's existence (ie - kubeapiserver is down) return the error
//   - if we get an error due to it not existing, disable the HCP controller
func shouldEnableHCP() (bool, error) {
	c, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return false, err
	}

	err = c.Get(context.TODO(), types.NamespacedName{Name: "hostedcontrolplanes.hypershift.openshift.io"}, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
