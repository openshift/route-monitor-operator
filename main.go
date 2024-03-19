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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configv1 "github.com/openshift/api/config/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	hypershiftv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	monitoringopenshiftiov1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
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
	utilruntime.Must(monitoringv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	utilruntime.Must(configv1.AddToScheme(scheme))
	utilruntime.Must(operatorv1.AddToScheme(scheme))
	utilruntime.Must(monitoringopenshiftiov1alpha1.AddToScheme(scheme))
	utilruntime.Must(hypershiftv1beta1.AddToScheme(scheme))
	utilruntime.Must(rhobsv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var enablehypershift bool
	var probeAddr string
	var managerConfigPath string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&enablehypershift, "enable-hypershift", false,
		"Enabling this for HyperShift")
	flag.StringVar(&managerConfigPath,
		"config",
		"",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.",
	)

	var blackboxExporterImage string
	var blackboxExporterNamespace string

	flag.StringVar(&blackboxExporterImage, "blackbox-image", "quay.io/prometheus/blackbox-exporter:master", "The image that will be used for the blackbox-exporter deployment")
	flag.StringVar(&blackboxExporterNamespace, "blackbox-namespace", "openshift-route-monitor-operator", "Blackbox-exporter deployment will reside on this Namespace")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "2793210b.openshift.io",
	}

	var err error

	if managerConfigPath != "" {
		cfgLoader := ctrl.ConfigFile().AtPath(managerConfigPath)
		if options, err = options.AndFrom(cfgLoader); err != nil {
			setupLog.Error(err, "Unable to load the manager config file")
			os.Exit(1)
		}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	routeMonitorReconciler := routemonitor.NewReconciler(mgr, blackboxExporterImage, blackboxExporterNamespace, enablehypershift)

	if err = routeMonitorReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RouteMonitor")
		os.Exit(1)
	}

	clusterUrlMonitorReconciler := clusterurlmonitor.NewReconciler(mgr, blackboxExporterImage, blackboxExporterNamespace, enablehypershift)

	if err = clusterUrlMonitorReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "clusterUrlMonitorReconciler")
		os.Exit(1)
	}

	enableHCP, err := shouldEnableHCP(mgr)
	if err != nil {
		setupLog.Error(err, "failed to determine whether HCP controller should be enabled", "controller", "HostedControlPlane")
	}
	if enableHCP {
		hostedControlPlaneReconciler := hostedcontrolplane.NewHostedControlPlaneReconciler(mgr, blackboxExporterNamespace)
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
func shouldEnableHCP(mgr ctrl.Manager) (bool, error) {
	c, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
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
