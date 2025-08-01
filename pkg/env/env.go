package env

import (
	"fmt"
	"slices"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// LoadSettings is a simple wrapper around godotenv.Load and env.Parse.
func LoadSettings[T any](filePaths ...string) (T, error) {
	filePaths = slices.DeleteFunc(filePaths, func(file string) bool {
		return file == ""
	})
	var settings T
	err := godotenv.Load(filePaths...)
	if err != nil {
		return settings, fmt.Errorf("failed to load settings from %s: %w", filePaths, err)
	}
	// Then override with environment variables
	if err := env.Parse(&settings); err != nil {
		return settings, fmt.Errorf("failed to parse settings from environment variables: %w", err)
	}

	return settings, nil
}
