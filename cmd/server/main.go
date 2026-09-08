package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"go.temporal.io/sdk/client"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/dani-zion/ganja-livre/internal/auth"
	"github.com/dani-zion/ganja-livre/internal/config"
	"github.com/dani-zion/ganja-livre/internal/graph/generated"
	"github.com/dani-zion/ganja-livre/internal/graph/resolvers"

	appmw "github.com/dani-zion/ganja-livre/internal/middleware"
	"github.com/dani-zion/ganja-livre/internal/mongodb"
)

func main() {
	// Load .env only in non-production environments
	if os.Getenv("APP_ENV") != "production" {
		_ = godotenv.Load()
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log := buildLogger(cfg.App.Env)
	defer log.Sync() //nolint:errcheck

	// ── MongoDB ──────────────────────────────────────────────────────────────
	mongoClient, err := mongodb.New(cfg.MongoDB, log)
	if err != nil {
		log.Fatal("failed to connect to MongoDB", zap.Error(err))
	}
	cols := mongoClient.Collections()

	// ── Temporal ─────────────────────────────────────────────────────────────
	temporalClient, err := client.Dial(client.Options{
		HostPort:  cfg.Temporal.HostPort,
		Namespace: cfg.Temporal.Namespace,
	})
	if err != nil {
		log.Fatal("failed to connect to Temporal", zap.Error(err))
	}

	// ── Services ─────────────────────────────────────────────────────────────
	jwtSvc := auth.NewService(cfg.JWT)
	resolver := resolvers.New(cols, jwtSvc, temporalClient)

	// ── GraphQL handler ──────────────────────────────────────────────────────
	gqlSrv := handler.New(generated.NewExecutableSchema(generated.Config{Resolvers: resolver}))

	// Only allow POST (disables GET introspection in production)
	gqlSrv.AddTransport(transport.POST{})
	gqlSrv.AddTransport(transport.Options{})

	// Disable introspection in production
	if cfg.App.Env != "production" {
		gqlSrv.Use(extension.Introspection{})
	}

	// Query complexity limit (DoS protection)
	gqlSrv.Use(extension.FixedComplexityLimit(100))

	// Persisted query cache
	gqlSrv.SetQueryCache(lru.New(1000))

	// ── Router ───────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(cfg.Server.ReadTimeout))
	r.Use(appmw.SecurityHeaders())
	r.Use(appmw.Auth(jwtSvc, log))

	r.Handle("/query", gqlSrv)

	// Playground only in non-production
	if cfg.App.Env != "production" {
		r.Handle("/playground", playground.Handler("Ganja Livre", "/query"))
		log.Info("GraphQL playground available at /playground")
	}

	// Health check endpoint
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// ── HTTP server ──────────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("Ganja Livre API starting", zap.String("port", cfg.Server.Port), zap.String("env", cfg.App.Env))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	log.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}

	temporalClient.Close()

	if err := mongoClient.Disconnect(ctx); err != nil {
		log.Error("failed to disconnect MongoDB", zap.Error(err))
	}

	log.Info("server stopped")
}

func buildLogger(env string) *zap.Logger {
	var cfg zap.Config
	if env == "production" {
		cfg = zap.NewProductionConfig()
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	} else {
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}
	log, _ := cfg.Build()
	return log
}
