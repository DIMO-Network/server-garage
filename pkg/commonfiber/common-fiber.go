package commonfiber

import (
	"context"
	"errors"
	"strings"

	"github.com/DIMO-Network/server-garage/pkg/richerrors"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// ContextLoggerMiddleware adds the http metadata to the logger and adds the logger to the context.
func ContextLoggerMiddleware(c *fiber.Ctx) error {
	ctx := c.UserContext()
	if ctx == context.Background() {
		// if the context is background, use the context from the request so we can get deadlines and cancellation signals
		ctx = c.Context()
	}
	newCtx := zerolog.Ctx(ctx).With().
		Str("httpMethod", c.Method()).
		Str("httpPath", strings.TrimPrefix(c.Path(), "/")).
		Str("sourceIp", getSourceIP(c)).
		Logger().
		WithContext(ctx)
	c.SetUserContext(newCtx)
	return c.Next()
}

func getSourceIP(c *fiber.Ctx) string {
	sourceIP := c.Get("X-Forwarded-For")
	if sourceIP == "" {
		sourceIP = c.Get("X-Real-IP")
	}
	if sourceIP == "" {
		sourceIP = c.IP()
	}
	return sourceIP
}

// ErrorHandler is a custom handler to log recovered errors using our logger and return json instead of string.
// This handler is aware of the richerrors package and will use the code and message from the error if available.
// It will also log the error to the set in the user context logger.
func ErrorHandler(ctx *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError // Default 500 statuscode
	message := "Internal error"

	var fiberErr *fiber.Error
	var richErr richerrors.Error
	if errors.As(err, &fiberErr) {
		code = fiberErr.Code
		message = fiberErr.Message
	} else if errors.As(err, &richErr) {
		message = richErr.ExternalMsg
		if richErr.Code != 0 {
			code = richErr.Code
		}
	}

	// log all errors except 404
	if code != fiber.StatusNotFound {
		logger := zerolog.Ctx(ctx.UserContext())
		logger.Err(err).Int("httpStatusCode", code).
			Msg("caught an error from http request")
	}

	return ctx.Status(code).JSON(CodedResponse{Code: code, Message: message})
}

// CodedResponse is a response that includes a code and a message.
type CodedResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}
