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

package clusterurlmonitor

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/route-monitor-operator/api/v1alpha1"
	monitoringv1alpha1 "github.com/openshift/route-monitor-operator/api/v1alpha1"
	"github.com/openshift/route-monitor-operator/pkg/blackboxexporter"
	utilreconcile "github.com/openshift/route-monitor-operator/pkg/util/reconcile"
)

// ClusterUrlMonitorReconciler reconciles a ClusterUrlMonitor object
type ClusterUrlMonitorReconciler struct {
	client.Client
	Log               logr.Logger
	Scheme            *runtime.Scheme
	BlackBoxImage     string
	BlackBoxNamespace string
}

const (
	FinalizerKey string = "clusterurlmonitor.monitoring.openshift.io/clusterurlmonitorcontroller"
)

// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.openshift.io,resources=clusterurlmonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=config.openshift.io,resources=ingresses,verbs=get;list;watch
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=prometheusrules,verbs=get;list;watch;create;delete

func (r *ClusterUrlMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("clusterurlmonitor", req.NamespacedName)

	clusterUrlMonitor, res, err := r.GetClusterUrlMonitor(req, ctx)
	if err != nil {
		return utilreconcile.RequeueWith(err)
	}

	if res.ShouldStop() {
		return utilreconcile.Stop()
	}

	blackboxExporter := blackboxexporter.New(r.Client, log, ctx, r.BlackBoxImage, r.BlackBoxNamespace)
	sup := NewSupplement(clusterUrlMonitor, r.Client, r.Log, blackboxExporter)

	return ProcessRequest(blackboxExporter, sup)
}

func (r *ClusterUrlMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&monitoringv1alpha1.ClusterUrlMonitor{}).
		Complete(r)
}

// GetClusterUrlMonitor return the ClusterUrlMonitor that is tested
func (r *ClusterUrlMonitorReconciler) GetClusterUrlMonitor(req ctrl.Request, ctx context.Context) (v1alpha1.ClusterUrlMonitor, utilreconcile.Result, error) {
	ClusterUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	err := r.Client.Get(ctx, req.NamespacedName, &ClusterUrlMonitor)
	if err != nil {
		// If this is an unknown error
		if !k8serrors.IsNotFound(err) {
			res, err := utilreconcile.RequeueReconcileWith(err)
			return v1alpha1.ClusterUrlMonitor{}, res, err
		}
		r.Log.V(2).Info("StopRequeue", "As ClusterUrlMonitor is 'NotFound', stopping requeue", nil)

		return v1alpha1.ClusterUrlMonitor{}, utilreconcile.StopOperation(), nil
	}

	// if the resource is empty, we should terminate
	emptyClustUrlMonitor := v1alpha1.ClusterUrlMonitor{}
	if reflect.DeepEqual(ClusterUrlMonitor, emptyClustUrlMonitor) {
		return v1alpha1.ClusterUrlMonitor{}, utilreconcile.StopOperation(), nil
	}

	return ClusterUrlMonitor, utilreconcile.ContinueOperation(), nil
}
