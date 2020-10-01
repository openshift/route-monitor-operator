package reconcile

import (
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

var Log logr.Logger = ctrl.Log.WithName("ReconcileOperation")

// Convert converts the ReconcileOperation to a 'ctrl.Result'
func (r Result) Convert() ctrl.Result {
	if !r.Continue {
		Log.V(1).Info("Info loss: converting 'ReconcileOperation' to 'ctrl.Result' with 'Continue'  as 'false'")
	}
	return ctrl.Result{
		Requeue:      r.Requeue,
		RequeueAfter: r.RequeueAfter,
	}
}

func (r Result) ReturnWith(err error) (ctrl.Result, error) {
	return r.Convert(), err
}

func Stop() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func RequeueWith(err error) (ctrl.Result, error) {
	return ctrl.Result{}, err
}

func Requeue() (ctrl.Result, error) {
	return ctrl.Result{Requeue: true}, nil
}
