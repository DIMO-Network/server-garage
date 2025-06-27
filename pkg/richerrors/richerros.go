package richerrors

import (
	"errors"
	"fmt"
)

// RichError is an error that contains a code, an external message, and a wrapped error.
type RichError struct {
	Code        int
	ExternalMsg string
	Err         error
}

// Error returns the ExternalMsg if it is set, otherwise it returns the error message of the wrapped error.
func (e RichError) Error() string {
	if e.ExternalMsg != "" {
		return e.ExternalMsg
	}
	return e.Err.Error()
}

// String implements the fmt.Stringer interface.
func (e RichError) String() string {
	return e.Error()
}

// MarshalText implements the encoding.TextMarshaler interface.
func (e RichError) MarshalText() ([]byte, error) {
	return []byte(e.Error()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (e *RichError) UnmarshalText(text []byte) error {
	errMsg := string(text)
	e.ExternalMsg = errMsg
	e.Err = errors.New(errMsg)
	return nil
}

// Unwrap returns the wrapped error to support errors.Is and errors.As.
func (e RichError) Unwrap() error {
	return e.Err
}

// Errorf creates a new RichError with the given external message and format.
func Errorf(externalMsg string, format string, args ...interface{}) RichError {
	return RichError{
		ExternalMsg: externalMsg,
		Err:         fmt.Errorf(format, args...),
	}
}

func ErrorWithCodef(code int, externalMsg string, format string, args ...interface{}) RichError {
	richErr := Errorf(externalMsg, format, args...)
	richErr.Code = code
	return richErr
}
