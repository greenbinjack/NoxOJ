package main

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"

	"noxoj/internal/cache"
	"noxoj/internal/config"
	"noxoj/internal/database"
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

// Handlers bundles the route handlers newRouter needs. Introduced
// this sprint because the parameter list was about to grow past
// register/login into refresh/logout too — a struct reads better
// than five positional http.HandlerFunc arguments and is easier to
// extend without reshuffling call sites every time a route is added.
type Handlers struct {
	Register http.HandlerFunc
	Login    http.HandlerFunc
	Refresh  http.HandlerFunc
	Logout   http.HandlerFunc
}

// newRouter takes readiness checks and handlers as plain functions,
// not concrete dependency-heavy types — so route tests that have
// nothing to do with the database or auth (TestRootRoute,
// TestHealthzRoute) never need live infrastructure just to construct
// a router.
func newRouter(
	logger zerolog.Logger,
	jwtSecret []byte,
	h Handlers,
	readinessChecks ...health.Checker,
) *chi.Mux {
	r := chi.NewRouter()
	r.Use(requestLogger(logger))

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("NoxOJ API — Sprint 1 skeleton is alive"))
	})

	r.Get("/healthz", health.Liveness)
	r.Get("/readyz", health.Readiness(readinessChecks...))

	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.Refresh)
	r.Post("/logout", h.Logout)

	// Minimal proof the auth chain (login -> cookie -> middleware ->
	// handler) actually works end to end. Not the real profile API —
	// that's Sprint 12; this just returns the authenticated user's ID.
	r.With(authmw.Authenticate(jwtSecret)).Get("/me", func(w http.ResponseWriter, r *http.Request) {
		userID, _ := authmw.UserIDFromContext(r.Context())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"user_id":"%s"}`, userID.String())
	})

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
	loginLimiter := ratelimit.NewLoginLimiter(5, 15*time.Minute)
	refreshTokens := tokenstore.NewRefreshTokenStore(redisClient)

	userHandler := handler.NewUserHandler(logger, users)
	authHandler := handler.NewAuthHandler(logger, users, cfg.JWTSecret, loginLimiter, refreshTokens, cfg.Environment)

	r := newRouter(logger, cfg.JWTSecret, Handlers{
		Register: userHandler.Register,
		Login:    authHandler.Login,
		Refresh:  authHandler.Refresh,
		Logout:   authHandler.Logout,
	}, database.Checker(db), cache.Checker(redisClient))

	addr := fmt.Sprintf(":%d", cfg.Port)
	logger.Info().Str("addr", addr).Str("environment", string(cfg.Environment)).Msg("NoxOJ API starting")

	if err := http.ListenAndServe(addr, r); err != nil {
		logger.Fatal().Err(err).Msg("server failed")
	}
}
