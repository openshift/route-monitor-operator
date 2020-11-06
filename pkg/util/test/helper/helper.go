package helper

import (
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
)

type MockHelper struct {
	CalledTimes   int
	ErrorResponse error
}

func NotFoundErrorHappensOnce() MockHelper {
	return MockHelper{
		CalledTimes:   1,
		ErrorResponse: consterror.NotFoundErr,
	}
}
func CustomErrorHappensOnce() MockHelper {
	return MockHelper{
		CalledTimes:   1,
		ErrorResponse: consterror.CustomError,
	}
}
