package reconcile

import "time"

type OperationResult struct {
	RequeueDelay   time.Duration
	RequeueRequest bool
	CancelRequest  bool
}

func StopProcessing() (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  true,
	}
	return
}

func RequeueWithError(errIn error) (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: true,
		CancelRequest:  false,
	}
	err = errIn
	return
}

func RequeueOnErrorOrStop(errIn error) (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  true,
	}
	err = errIn
	return
}

func RequeueOnErrorOrContinue(errIn error) (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  false,
	}
	err = errIn
	return
}

func RequeueAfter(delay time.Duration, errIn error) (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   delay,
		RequeueRequest: true,
		CancelRequest:  false,
	}
	err = errIn
	return
}

func ContinueProcessing() (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  false,
	}
	return
}
