package utils

import "time"

type OperationResult struct {
	RequeueDelay   time.Duration
	RequeueRequest bool
	CancelRequest  bool
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

func RequeueWithError(errIn error) (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: true,
		CancelRequest:  false,
	}
	err = errIn
	return
}

func Requeue() (result OperationResult, err error) {
	result = OperationResult{
		RequeueDelay:   0,
		RequeueRequest: true,
		CancelRequest:  false,
	}
	return
}

func ContinueOperationResult() OperationResult {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  false,
	}
}

func ContinueProcessing() (result OperationResult, err error) {
	result = ContinueOperationResult()
	return
}

func StopProcessing() (result OperationResult, err error) {
	result = StopOperationResult()
	return
}

func StopOperationResult() OperationResult {
	return OperationResult{
		RequeueDelay:   0,
		RequeueRequest: false,
		CancelRequest:  true,
	}
}
