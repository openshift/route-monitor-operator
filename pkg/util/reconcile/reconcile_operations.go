package reconcile

import (
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
)

var Log logr.Logger = ctrl.Log.WithName("ReconcileOperation")

type ReconcileOperation struct {
	Requeue        bool
	StopProcessing bool
	RequeueAfter   time.Duration
}

func StopOperation() ReconcileOperation {
	return ReconcileOperation{
		StopProcessing: true,
	}
}

func RequeueOperation() ReconcileOperation {
	return ReconcileOperation{
		Requeue: true,
	}
}

func ContinueOperation() ReconcileOperation {
	return ReconcileOperation{}
}

func StopProcessingResponse() (result ReconcileOperation, err error) {
	result = StopOperation()
	return
}

func RequeueResponseWith(errIn error) (result ReconcileOperation, err error) {
	if errIn != nil {
		err = errIn
		result = RequeueOperation()
		return
	}
	return
}

func RequeueResponseAfter(delay time.Duration, errIn error) (result ReconcileOperation, err error) {
	result, err = RequeueResponseWith(errIn)
	if delay != time.Duration(0) {
		result.RequeueAfter = delay
		return
	}
	return
}

func ContinueProcessingResponse() (result ReconcileOperation, err error) {
	return
}

func (reconcileOperation ReconcileOperation) ToResultWith(err error) (ctrl.Result, error) {
	return reconcileOperation.ToResult(), err
}

// ToResult converts the ReconcileOperation to a 'ctrl.Result'
func (reconcileOperation ReconcileOperation) ToResult() ctrl.Result {
	if reconcileOperation.StopProcessing {
		Log.V(1).Info("Info loss: converting 'ReconcileOperation' to 'ctrl.Result' with StopProcessing as 'true'")
	}
	return ctrl.Result{
		Requeue:      reconcileOperation.Requeue,
		RequeueAfter: reconcileOperation.RequeueAfter,
	}
}

func StopProcessingResult() (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func RequeueResultWith(err error) (ctrl.Result, error) {
	res := ctrl.Result{
		Requeue: true,
	}
	return res, err
}

func RequeueResult() (ctrl.Result, error) {
	return RequeueResultWith(nil)
}
