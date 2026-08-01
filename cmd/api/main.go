package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/rs/zerolog"

	"noxoj/internal/cache"
	"noxoj/internal/config"
	"noxoj/internal/database"
	"noxoj/internal/domain"
	"noxoj/internal/handler"
	"noxoj/internal/health"
	"noxoj/internal/logging"
	authmw "noxoj/internal/middleware"
	"noxoj/internal/ratelimit"
	"noxoj/internal/repository"
	"noxoj/internal/tokenstore"
)

func requestLogger(logger zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			next.ServeHTTP(ww, r)

			logger.Info().
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", ww.Status()).
				Dur("duration", time.Since(start)).
				Msg("request handled")
		})
	}
}

// Handlers bundles the route handlers newRouter needs — a struct
// instead of a growing pile of positional http.HandlerFunc arguments.
type Handlers struct {
	Register             http.HandlerFunc
	Login                http.HandlerFunc
	Refresh              http.HandlerFunc
	Logout               http.HandlerFunc
	Me                   http.HandlerFunc
	RequestPasswordReset http.HandlerFunc
	ConfirmPasswordReset http.HandlerFunc
	DeactivateUser       http.HandlerFunc
}

// newRouter takes readiness checks and handlers as plain functions,
// not concrete dependency-heavy types — so route tests that have
// nothing to do with the database or auth (TestRootRoute,
// TestHealthzRoute) never need live infrastructure just to construct
// a router.
func newRouter(
	logger zerolog.Logger,
	jwtSecret []byte,
	corsAllowedOrigin string,
	h Handlers,
	readinessChecks ...health.Checker,
) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))
	// AllowCredentials must be true (cookies carry the session) and
	// AllowedOrigins must be an exact origin, never "*" — browsers
	// reject a wildcard outright once credentials are involved.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{corsAllowedOrigin},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: true,
	}))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NoxOJ API — Sprint 1 skeleton is alive"))
	})

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness(readinessChecks...))

	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.Refresh)
	r.Post("/logout", h.Logout)
	r.Post("/password-reset/request", h.RequestPasswordReset)
	r.Post("/password-reset/confirm", h.ConfirmPasswordReset)

	r.With(authmw.Authenticate(jwtSecret)).Get("/users/me", h.Me)

	// Minimal proof RBAC itself works — admin-only, nothing behind it
	// yet worth protecting for real (that starts once Problem/Contest
	// management exist).
	r.With(authmw.Authenticate(jwtSecret), authmw.RequireRole(domain.RoleAdmin)).Get("/admin/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok, you are an admin"}`))
	})

	r.With(authmw.Authenticate(jwtSecret), authmw.RequireRole(domain.RoleAdmin)).Post("/admin/users/{id}/deactivate", h.DeactivateUser)

	return r
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger := logging.New(cfg.Environment)

	db, err := database.Connect(database.Config{
		Host:     cfg.Postgres.Host,
		Port:     cfg.Postgres.Port,
		User:     cfg.Postgres.User,
		Password: cfg.Postgres.Password,
		Name:     cfg.Postgres.Name,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	redisClient, err := cache.Connect(cache.Config{
		Host: cfg.Redis.Host,
		Port: cfg.Redis.Port,
	})
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to connect to redis")
	}
	defer redisClient.Close()

	users := repository.NewUserRepository(db)
	roles := repository.NewRoleRepository(db)
	loginLimiter := ratelimit.NewLoginLimiter(5, 15*time.Minute)
	refreshTokens := tokenstore.NewRefreshTokenStore(redisClient)
	resetTokens := tokenstore.NewPasswordResetTokenStore(redisClient)

	userHandler := handler.NewUserHandler(logger, users, refreshTokens)
	authHandler := handler.NewAuthHandler(logger, users, roles, cfg.JWTSecret, loginLimiter, refreshTokens, resetTokens, cfg.Environment)

	r := newRouter(logger, cfg.JWTSecret, cfg.CORSAllowedOrigin, Handlers{
		Register:             userHandler.Register,
		Login:                authHandler.Login,
		Refresh:              authHandler.Refresh,
		Logout:               authHandler.Logout,
		Me:                   userHandler.Me,
		RequestPasswordReset: authHandler.RequestPasswordReset,
		ConfirmPasswordReset: authHandler.ConfirmPasswordReset,
		DeactivateUser:       userHandler.Deactivate,
	}, database.Checker(db), cache.Checker(redisClient))

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info().Str("addr", addr).Str("environment", string(cfg.Environment)).Msg("NoxOJ API starting")

	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
