package env

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

// LoadSettings is a simple wrapper around godotenv.Load and env.Parse.
func LoadSettings[T any](filePath ...string) (T, error) {
	var settings T
	err := godotenv.Load(filePath...)
	if err != nil {
		return settings, fmt.Errorf("failed to load settings from %s: %w", filePath, err)
	}
	// Then override with environment variables
	if err := env.Parse(&settings); err != nil {
		return settings, fmt.Errorf("failed to parse settings from environment variables: %w", err)
	}

	return settings, nil
}
