// Command api boots the league simulator service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/config"
	"github.com/iberkayC/case1back/internal/external/openai"
	"github.com/iberkayC/case1back/internal/httpapi"
	"github.com/iberkayC/case1back/internal/platform/random"
	"github.com/iberkayC/case1back/internal/predict/aianalyst"
	"github.com/iberkayC/case1back/internal/predict/montecarlo"
	"github.com/iberkayC/case1back/internal/predict/validating"
	mysqlstore "github.com/iberkayC/case1back/internal/storage/mysql"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", slog.Any("err", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config.Load: %w", err)
	}
	setupLogger(cfg.App.LogLevel)
	slog.Info("config loaded", slog.String("env", cfg.App.Env), slog.Int("port", cfg.HTTP.Port))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := mysqlstore.Open(ctx, cfg.MySQL.DSN, mysqlstore.Options{
		MaxOpenConns:    cfg.MySQL.MaxOpenConns,
		MaxIdleConns:    cfg.MySQL.MaxIdleConns,
		ConnMaxLifetime: cfg.MySQL.ConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("mysql.Open: %w", err)
	}
	defer func() {
		if cErr := store.Close(); cErr != nil {
			slog.Warn("mysql close", slog.Any("err", cErr))
		}
	}()

	leagueRepo := store.Leagues()
	matchRepo := store.Matches()
	snapshotRepo := store.Snapshots()
	predictionRepo := store.Predictions()
	idempotencyRepo := store.Idempotency()

	leagueQueries := app.NewLeagueQueryService(leagueRepo, matchRepo)

	predictors := []app.Predictor{
		validating.New(montecarlo.New(cfg.Predict.MonteCarloSimulations)),
	}
	if cfg.OpenAI.Enabled() {
		client, err := openai.New(openai.Options{
			APIKey:  cfg.OpenAI.APIKey,
			Model:   cfg.OpenAI.Model,
			BaseURL: cfg.OpenAI.BaseURL,
			Timeout: cfg.OpenAI.Timeout,
		})
		if err != nil {
			return fmt.Errorf("openai.New: %w", err)
		}
		predictors = append(predictors, validating.New(aianalyst.New(client, cfg.OpenAI.Model)))
		slog.Info("ai_analyst predictor enabled", slog.String("model", cfg.OpenAI.Model))
	} else {
		slog.Info("ai_analyst predictor disabled (OPENAI_API_KEY unset)")
	}

	predictionService := app.NewPredictionService(
		leagueRepo, matchRepo, snapshotRepo, predictionRepo, store, predictors,
	)
	leagueCommands := app.NewLeagueCommandService(leagueRepo, matchRepo, store, predictionService.MarkPriorRunsStale)
	seasonAdvancer := app.NewSeasonAdvancer(leagueRepo, matchRepo, store, random.NewDefault(), predictionService.MarkPriorRunsStale)
	idempotencyService := app.NewIdempotencyService(store, idempotencyRepo)

	handlers := &httpapi.Handlers{
		Queries:    leagueQueries,
		Commands:   leagueCommands,
		Seasons:    seasonAdvancer,
		Prediction: predictionService,
	}
	router := httpapi.NewRouter(httpapi.RouterDeps{
		Handlers:    handlers,
		Idempotency: idempotencyService,
		Config:      cfg,
	})

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.HTTP.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("http listening", slog.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.App.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	slog.Info("shutdown complete")
	return nil
}

func setupLogger(levelStr string) {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "info", "":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
}
