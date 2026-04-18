package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"github.com/batuhan/marketintel/internal/auth"
	"github.com/batuhan/marketintel/internal/config"
	"github.com/batuhan/marketintel/internal/dashboard"
	"github.com/batuhan/marketintel/internal/datasource"
	"github.com/batuhan/marketintel/internal/discovery"
	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ailog"
	"github.com/batuhan/marketintel/internal/platform/db"
	"github.com/batuhan/marketintel/internal/scoring"
	"github.com/batuhan/marketintel/internal/server"
	"github.com/batuhan/marketintel/migrations"
)

func main() {
	_ = godotenv.Load()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel("INFO"),
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseLogLevel(cfg.LogLevel),
	})))

	if err := cfg.Validate(); err != nil {
		slog.Warn("config validation", "warning", err)
	}

	slog.Info("starting server",
		"app", cfg.AppName,
		"env", cfg.AppEnv,
		"port", cfg.Port,
	)

	// Database
	ctx := context.Background()
	pool, err := db.NewPool(ctx, cfg.PostgresDSN())
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Migrations
	if err := db.RunMigrations(cfg.PostgresDSN(), migrations.FS()); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Auth module
	userRepo := auth.NewUserRepository(pool)
	jwtMgr := auth.NewJWTManager(cfg.SecretKey, cfg.AccessTokenExpireMinutes, cfg.RefreshTokenExpireDays)
	authSvc := auth.NewService(userRepo, jwtMgr)
	authHandler := auth.NewHandler(authSvc)
	userFetcher := auth.NewUserFetcherAdapter(userRepo)

	// AI providers
	providers := make(map[string]ai.Provider)
	if cfg.AzureOpenAIAPIKey != "" && cfg.AzureOpenAIEndpoint != "" {
		providers["azure_openai"] = ai.NewAzureOpenAI(cfg.AzureOpenAIAPIKey, cfg.AzureOpenAIEndpoint)
		slog.Info("ai: azure_openai configured")
	}
	if cfg.AnthropicAPIKey != "" {
		providers["anthropic"] = ai.NewClaude(cfg.AnthropicAPIKey)
		slog.Info("ai: anthropic configured")
	}
	if cfg.GoogleAIAPIKey != "" {
		providers["google"] = ai.NewGemini(cfg.GoogleAIAPIKey)
		slog.Info("ai: google configured")
	}

	var aiCache *ai.Cache
	if cfg.RedisURL != "" {
		c, err := ai.NewCache(cfg.RedisURL)
		if err != nil {
			slog.Warn("ai cache unavailable", "error", err)
		} else {
			aiCache = c
			defer c.Close()
		}
	}

	aiRouter := ai.NewRouter(providers, ai.DefaultTasks(), aiCache)

	// AI request logging
	aiLogRepo := ailog.NewRepository(pool)
	aiRouter.SetLogFunc(func(ctx context.Context, result *ai.AIResult, userID, searchID *uuid.UUID) {
		log := &domain.AIRequestLog{
			ID:          uuid.New(),
			UserID:      userID,
			SearchID:    searchID,
			RequestType: result.RequestType,
			Provider:    result.Provider,
			Model:       result.Model,
			InputTokens: &result.InputTokens,
			OutputTokens: &result.OutputTokens,
			CostUSD:     &result.CostUSD,
			LatencyMS:   &result.LatencyMS,
			CacheHit:    result.CacheHit,
		}
		if err := aiLogRepo.Create(ctx, log); err != nil {
			slog.Warn("failed to log AI request", "error", err)
		}
	})

	// Data source connectors
	var places domain.BusinessFinder
	if cfg.GooglePlacesAPIKey != "" {
		places = datasource.NewGooglePlaces(cfg.GooglePlacesAPIKey)
		slog.Info("datasource: google places configured")
	}

	// Scoring module
	scoringRepo := scoring.NewRepository(pool)

	// Discovery module — Excel-only flow
	discRepo := discovery.NewRepository(pool)
	classifier := discovery.NewClassifier(aiRouter)
	scorer := discovery.NewScorer(aiRouter)
	pipeline := discovery.NewPipeline(aiRouter, places, classifier, scorer, discRepo)
	discoveryHandler := discovery.NewHandler(pipeline, discRepo)
	scoringHandler := scoring.NewHandler(scoringRepo)

	// Dashboard module
	dashboardHandler := dashboard.NewHandler(pool, aiLogRepo)

	// Server
	srv := server.New(jwtMgr, userFetcher, authHandler, discoveryHandler, scoringHandler, dashboardHandler, cfg.ParsedCORSOrigins())

	httpSrv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("listening", "addr", httpSrv.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	slog.Info("shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		slog.Error("forced shutdown", "error", err)
	}

	pool.Close()
	slog.Info("server stopped")
}

func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

