// Package database manages NoxOJ's connection to PostgreSQL.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver with database/sql
)

// Config holds what's needed to reach Postgres. Kept separate from
// config.Config so this package doesn't need to know about the rest
// of the app's settings — just the ones it actually uses.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
}

func (c Config) dsn() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.User, c.Password, c.Host, c.Port, c.Name)
}

// Connect opens a pooled connection to Postgres and verifies it's
// actually reachable (sqlx.Connect pings under the hood) before
// returning — a returned *sqlx.DB is a promise the database was
// reachable at least at startup, not just that a DSN parsed cleanly.
func Connect(cfg Config) (*sqlx.DB, error) {
	db, err := sqlx.Connect("pgx", cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("connecting to postgres: %w", err)
	}

	// Pool sizing: matches the API service's own concurrency ceiling,
	// not an arbitrary number — no point letting more requests queue
	// for a DB connection than the app would sensibly handle at once.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// Checker returns a health.Checker (see internal/health) that reports
// whether the database is currently reachable — used to give /readyz
// something real to check, closing the gap Sprint 6 left open.
func Checker(db *sqlx.DB) func() error {
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return db.PingContext(ctx)
	}
}
