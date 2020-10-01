package reconcile

import "time"

type Result struct {
	Requeue      bool
	Continue     bool
	RequeueAfter time.Duration
}

func StopOperation() Result {
	return Result{}
}

func ContinueOperation() Result {
	return Result{
		Continue: true,
	}
}

func StopReconcile() (result Result, err error) {
	result = StopOperation()
	return
}

// RequeueReconcileWith will not requeue if errIn doesn't fire
func RequeueReconcileWith(errIn error) (result Result, err error) {
	if errIn != nil {
		err = errIn
		return
	}
	return
}

func ContinueReconcile() (result Result, err error) {
	result = ContinueOperation()
	return
}
