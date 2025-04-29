package errors

import (
	"errors"
)

var (
	ErrNoHost     = errors.New("no Host: extracted RouteURL is empty")
	ErrInvalidSLO = errors.New("invalid RawSlo: string cannot be parsed " +
		"or is not in correct range, or type is not supported")
	ErrInvalidReferenceUpdate = errors.New("invalid Reference Update: currently the reference cannot be changed in flight, " +
		"please delete the parent resource and create it in the new name")
)
