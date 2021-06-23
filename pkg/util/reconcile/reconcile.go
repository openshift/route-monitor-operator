package reconcile

import "time"

type Result struct {
	Requeue bool
	// Continue is used mostly by ShouldStop() and it's named this way so the empty Result will stop proccesing
	Continue     bool
	RequeueAfter time.Duration
}

func (r Result) RequeueOrStop() bool {
	return r.Requeue || !r.Continue
}

func (r Result) ShouldStop() bool {
	return !r.Continue
}

func StopOperation() Result {
	return Result{}
}

func RequeueOperation() Result {
	return Result{
		Requeue: true,
	}
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

func RequeueReconcile() (result Result, err error) {
	result = RequeueOperation()
	return
}

// RequeueReconcileWith will not requeue if errIn doesn't fire
func RequeueReconcileWith(errIn error) (result Result, err error) {
	if errIn != nil {
		result.Requeue = true
		err = errIn
		return
	}
	return
}

func ContinueReconcile() (result Result, err error) {
	result = ContinueOperation()
	return
}
