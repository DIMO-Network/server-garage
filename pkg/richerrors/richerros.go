package richerrors

import (
	"errors"
	"fmt"
)

// Error is an error that contains a code, an external message, and a wrapped error.
type Error struct {
	Code        int
	ExternalMsg string
	Err         error
}

// Error returns the ExternalMsg if it is set, otherwise it returns the error message of the wrapped error.
func (e Error) Error() string {
	if e.ExternalMsg != "" && e.Err != nil {
		return fmt.Sprintf("%s: %s", e.ExternalMsg, e.Err.Error())
	}
	if e.ExternalMsg != "" {
		return e.ExternalMsg
	}
	return e.Err.Error()
}

// String implements the fmt.Stringer interface.
func (e Error) String() string {
	return e.Error()
}

// MarshalText implements the encoding.TextMarshaler interface.
func (e Error) MarshalText() ([]byte, error) {
	return []byte(e.Error()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface.
func (e *Error) UnmarshalText(text []byte) error {
	errMsg := string(text)
	e.ExternalMsg = errMsg
	e.Err = errors.New(errMsg)
	return nil
}

// Unwrap returns the wrapped error to support errors.Is and errors.As.
func (e Error) Unwrap() error {
	return e.Err
}

// Errorf creates a new RichError with the given external message and format.
func Errorf(externalMsg string, format string, args ...interface{}) Error {
	return Error{
		ExternalMsg: externalMsg,
		Err:         fmt.Errorf(format, args...),
	}
}

func ErrorWithCodef(code int, externalMsg string, format string, args ...interface{}) Error {
	richErr := Errorf(externalMsg, format, args...)
	richErr.Code = code
	return richErr
}

// IsRichError checks if the error wraps a RichError.
func IsRichError(err error) bool {
	return errors.As(err, &Error{})
}

// AsRichError unwraps the error if it wraps a RichError.
func AsRichError(err error) (Error, bool) {
	var richErr Error
	if errors.As(err, &richErr) {
		return richErr, true
	}
	return Error{}, false
}
