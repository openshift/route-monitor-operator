package helper

import (
	consterror "github.com/openshift/route-monitor-operator/pkg/consts/test/error"
)

// MockHelper is a struct that makes using mocks easier
// without MockHelper we needed to have two fields on every fuction we wanted to mock
//
// for example:
//
// var (
// 	getCalledTimes int
// 	getErrorResponse error
// )
// BeforeEach(func() {
// 	getCalledTimes = 0
// 	getErrorResponse = nil
// }
// JustBeforeEach(func() {
// 	mockClient.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
// 		Return(getErrorResponse).
// 		Times(getCalledTimes)
// }
//
// this made the tests full of variables that were used seperately
// by joining them into one struct, a more unified way was introduced
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

type ResourceComparerMockHelper struct {
	CalledTimes int
	ReturnValue bool
}
