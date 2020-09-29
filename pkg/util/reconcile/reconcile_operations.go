package reconcile

import (
	ctrl "sigs.k8s.io/controller-runtime"
	"time"
)

func StopProcessing() (result ctrl.Result, err error) {
	result = ctrl.Result{
		Requeue: false,
	}
	return
}

func RequeueWith(errIn error) (result ctrl.Result, err error) {
	result = ctrl.Result{
		Requeue: true,
	}
	err = errIn
	return
}

func RequeueAfter(delay time.Duration, errIn error) (result ctrl.Result, err error) {
	result = ctrl.Result{
		Requeue:      true,
		RequeueAfter: delay,
	}
	err = errIn
	return
}

func ContinueProcessing() (result ctrl.Result, err error) {
	result = ctrl.Result{}
	return
}
