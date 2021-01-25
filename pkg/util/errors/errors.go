package errors

import (
	"errors"
)

var (
	NoHost     = errors.New("No Host: extracted RouteURL is empty")
	InvalidSLO = errors.New("Invalid RawSlo: string cannot be parsed or is not in correct range, or type is not supported")
)
