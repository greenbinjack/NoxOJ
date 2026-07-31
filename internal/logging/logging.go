// Package logging configures NoxOJ's structured logger.
package logging

import (
	"os"

	"github.com/rs/zerolog"

	"noxoj/internal/config"
)

// New returns a zerolog.Logger configured for the given environment.
// Development gets a human-readable console format, since a person is
// likely watching it live; everything else gets strict JSON, since
// that's what a log aggregator in staging/production actually parses.
func New(env config.Environment) zerolog.Logger {
	if env == config.Development {
		return zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	}
	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}
