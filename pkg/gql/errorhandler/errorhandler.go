package errorhandler

import (
	"context"
	"errors"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/rs/zerolog"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// ErrorPresenter is a custom error presenter that logs the error and returns a gqlerror.Error.
func ErrorPresenter(ctx context.Context, err error) *gqlerror.Error {
	if err == nil {
		return nil
	}
	var gqlErr *gqlerror.Error
	if !errors.As(err, &gqlErr) {
		// If someone incorrectly returns a raw error, do not expose the error message.
		gqlErr = gqlerror.WrapPath(graphql.GetPath(ctx), err)
		gqlErr.Message = "internal server error"
	}
	zerolog.Ctx(ctx).Error().
		Err(gqlErr.Err).
		Str("gqlPath", gqlErr.Path.String()).
		Fields(gqlErr.Extensions).
		Msg(gqlErr.Message)
	return gqlErr
}

// NewErrorWithMsg creates a new gqlerror.Error with a message and code.
func NewErrorWithMsg(ctx context.Context, err error, message string, code string) *gqlerror.Error {
	return &gqlerror.Error{
		Err:     err,
		Message: message,
		Path:    graphql.GetPath(ctx),
		Extensions: map[string]interface{}{
			"reason": http.StatusText(http.StatusInternalServerError),
			"code":   code,
		},
	}
}

// NewInternalErrorWithMsg creates a new internal server error with a message.
func NewInternalErrorWithMsg(ctx context.Context, err error, message string) *gqlerror.Error {
	return NewErrorWithMsg(ctx, err, message, CodeInternalServerError)
}

// NewBadRequestErrorWithMsg creates a new bad request error with a message.
func NewBadRequestErrorWithMsg(ctx context.Context, err error, message string) *gqlerror.Error {
	return NewErrorWithMsg(ctx, err, message, CodeBadRequest)
}

// NewBadRequestError creates a new bad request error.
func NewBadRequestError(ctx context.Context, err error) *gqlerror.Error {
	return NewBadRequestErrorWithMsg(ctx, err, err.Error())
}

// NewUnauthorizedErrorWithMsg creates a new unauthorized error with a message.
func NewUnauthorizedErrorWithMsg(ctx context.Context, err error, message string) *gqlerror.Error {
	return NewErrorWithMsg(ctx, err, message, CodeUnauthorized)
}

// NewUnauthorizedError creates a new unauthorized error.
func NewUnauthorizedError(ctx context.Context, err error) *gqlerror.Error {
	return NewUnauthorizedErrorWithMsg(ctx, err, err.Error())
}

// ErrCode returns the code of the gqlerror.Error
// If the code is not correctly set, it returns an empty string.
func ErrCode(gqlErr *gqlerror.Error) string {
	if gqlErr == nil || gqlErr.Extensions == nil {
		return ""
	}
	code, ok := gqlErr.Extensions["code"]
	if !ok {
		return ""
	}
	codeStr, ok := code.(string)
	if !ok {
		return ""
	}
	return codeStr
}

// IsErrCode checks if the error is a gqlerror.Error and has the given code.
func IsErrCode(err error, code string) bool {
	var gqlErr *gqlerror.Error
	if !errors.As(err, &gqlErr) {
		return false
	}
	return ErrCode(gqlErr) == code
}

// HasErrCode checks if the gqlerror.List contains an error with the given code.
func HasErrCode(errs *gqlerror.List, code string) bool {
	for _, err := range *errs {
		if IsErrCode(err, code) {
			return true
		}
	}
	return false
}
