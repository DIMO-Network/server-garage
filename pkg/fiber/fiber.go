package fiber

import (
	"context"

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
	newCtx := zerolog.Ctx(ctx).With().Str("httpMethod", c.Method()).Str("httpPath", c.Path()).Str("sourceIp", getSourceIP(c)).Logger().WithContext(ctx)
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
