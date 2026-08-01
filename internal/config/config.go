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

// insecureDefaultJWTSecret is intentionally obvious and never valid
// outside local development — Load refuses to start in production
// with this value still set, so a forgotten JWT_SECRET fails loudly
// at startup instead of silently signing every token with a secret
// anyone can read directly out of this file.
const insecureDefaultJWTSecret = "insecure-dev-secret-change-in-production"

// Config holds NoxOJ's runtime settings.
type Config struct {
	Environment Environment
	Port        int
	Postgres    PostgresConfig
	JWTSecret   []byte
}

// PostgresConfig holds what's needed to reach the database. Host
// defaults to "localhost" for running the app bare-metal against a
// docker-compose Postgres on its published port; the containerized
// api service overrides it to "postgres" (the Compose service name)
// via docker-compose.yml's environment block — same config, same
// code, different value per deployment, exactly the environment
// parity principle this package exists to enforce.
type PostgresConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

// Load reads config from (in increasing priority) built-in defaults,
// a local .env file if one exists, and real environment variables.
func Load() (*Config, error) {
	v := viper.New()

	v.SetDefault("ENVIRONMENT", string(Development))
	v.SetDefault("PORT", 8081)
	v.SetDefault("POSTGRES_HOST", "localhost")
	v.SetDefault("POSTGRES_PORT", 5432)
	v.SetDefault("POSTGRES_USER", "noxoj")
	v.SetDefault("POSTGRES_PASSWORD", "noxoj_dev_password")
	v.SetDefault("POSTGRES_DB", "noxoj")
	v.SetDefault("JWT_SECRET", insecureDefaultJWTSecret)

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

	jwtSecret := v.GetString("JWT_SECRET")
	if env == Production && jwtSecret == insecureDefaultJWTSecret {
		return nil, errors.New("JWT_SECRET must be set explicitly in production — refusing to start with the default dev value")
	}

	return &Config{
		Environment: env,
		Port:        v.GetInt("PORT"),
		Postgres: PostgresConfig{
			Host:     v.GetString("POSTGRES_HOST"),
			Port:     v.GetInt("POSTGRES_PORT"),
			User:     v.GetString("POSTGRES_USER"),
			Password: v.GetString("POSTGRES_PASSWORD"),
			Name:     v.GetString("POSTGRES_DB"),
		},
		JWTSecret: []byte(jwtSecret),
	}, nil
}
