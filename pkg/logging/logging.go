package logging

import (
	"io"
	"os"
	"runtime/debug"

	"github.com/rs/zerolog"
)

// GetAndSetDefaultLogger gets the default logger and sets it to the default context logger.
// It also adds the app name and the commit hash to the logger.
func GetAndSetDefaultLogger(appName string) zerolog.Logger {
	return GetAndSetDefaultLoggerWithWriter(appName, os.Stdout)
}

// GetAndSetDefaultLogger gets the default logger and sets it to the default context logger.
// It also adds the app name and the commit hash to the logger.
func GetAndSetDefaultLoggerWithWriter(appName string, writer io.Writer) zerolog.Logger {
	logger := zerolog.New(writer).With().Timestamp().Str("app", appName).Logger()
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) == 40 {
				logger = logger.With().Str("commit", s.Value[:7]).Logger()
				break
			}
		}
	}
	zerolog.DefaultContextLogger = &logger
	return logger
}
