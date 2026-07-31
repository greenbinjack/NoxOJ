// Package config loads NoxOJ's runtime configuration from a local .env
// file (optional, for dev convenience) and real environment variables
// (always take precedence — the actual 12-factor source of truth).
package config

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/viper"
)

// Environment identifies which deployment environment the app is
// running in. Every environment runs the same code — only these
// values are allowed to differ.
type Environment string

const (
	Development Environment = "development"
	Test        Environment = "test"
	Production  Environment = "production"
)

func (e Environment) validate() error {
	switch e {
	case Development, Test, Production:
		return nil
	default:
		return fmt.Errorf("invalid ENVIRONMENT %q: must be one of development, test, production", e)
	}
}

// Config holds NoxOJ's runtime settings.
type Config struct {
	Environment Environment
	Port        int
}

// Load reads config from (in increasing priority) built-in defaults,
// a local .env file if one exists, and real environment variables.
func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("ENVIRONMENT", string(Development))
	v.SetDefault("PORT", 8081)

	v.SetConfigFile(".env")
	v.SetConfigType("env")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("reading .env: %w", err)
		}
		// No .env file — fine, defaults and real env vars still apply.
	}

	v.AutomaticEnv()

	env := Environment(v.GetString("ENVIRONMENT"))
	if err := env.validate(); err != nil {
		return nil, err
	}

	return &Config{
		Environment: env,
		Port:        v.GetInt("PORT"),
	}, nil
}
