package errors

import (
	"errors"
)

var (
	NoHost = errors.New("No Host: extracted RouteURL is empty")
)
