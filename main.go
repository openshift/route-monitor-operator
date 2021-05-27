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

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	configv1 "github.com/openshift/api/config/v1"
	routev1 "github.com/openshift/api/route/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	monitoringopenshiftiov1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/controllers/routemonitor"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
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

	utilruntime.Must(monitoringopenshiftiov1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	var blackboxExporterImage string
	var blackboxExporterNamespace string

	flag.StringVar(&blackboxExporterImage, "blackbox-image", "quay.io/app-sre/prom-blackbox-exporter:master", "The image that will be used for the blackbox-exporter deployment")
	flag.StringVar(&blackboxExporterNamespace, "blackbox-namespace", "openshift-route-monitor-operator", "Blackbox-exporter deployment will reside on this Namespace")

	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "2793210b.openshift.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	routeMonitorReconciler := &routemonitor.RouteMonitorReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("RouteMonitor"),
		Scheme: mgr.GetScheme(),
	}
	//routeMonitorReconciler.RouteMonitorSupplement = routemonitor.NewRouteMonitorSupplement(*routeMonitorReconciler)
	routeMonitorReconciler.BlackBoxExporter = blackboxexporter.New(routeMonitorReconciler.Client,
		routeMonitorReconciler.Log, context.Background(), blackboxExporterImage, blackboxExporterNamespace)

	if err = routeMonitorReconciler.SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RouteMonitor")
		os.Exit(1)
	}

	/*
		if err = (&clusterurlmonitor.ClusterUrlMonitorReconciler{
			Client:                    mgr.GetClient(),
			Log:                       ctrl.Log.WithName("controllers").WithName("ClusterUrlMonitor"),
			Scheme:                    mgr.GetScheme(),
			BlackBoxExporterImage:     blackboxExporterImage,
			BlackBoxExporterNamespace: blackboxExporterNamespace,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ClusterUrlMonitor")
			os.Exit(1)
		}*/
	// +kubebuilder:scaffold:builder

	setupLog.V(2).Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

	setupLog.V(1).Info("`mgr.Start` is blocking:",
		"so this message (or anything that resides after it) won't execute until teardown", nil)
}
