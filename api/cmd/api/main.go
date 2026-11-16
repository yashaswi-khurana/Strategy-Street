package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/strategy-street/api/internal/config"
	"github.com/strategy-street/api/internal/httpx"
)

type liveReadinessChecker struct {
	dbPool      *pgxpool.Pool
	redisClient *redis.Client
}

func (c *liveReadinessChecker) PingDatabase(ctx context.Context) error {
	if c.dbPool == nil {
		return errors.New("database connection pool not initialized")
	}
	return c.dbPool.Ping(ctx)
}

func (c *liveReadinessChecker) PingRedis(ctx context.Context) error {
	if c.redisClient == nil {
		return errors.New("redis client not initialized")
	}
	return c.redisClient.Ping(ctx).Err()
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fallbackLogger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		fallbackLogger.Error("failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger := httpx.NewJSONLogger(os.Stdout, cfg.LogLevel)
	slog.SetDefault(logger)

	logger.Info("starting strategy street api service",
		slog.String("env", cfg.Env),
		slog.String("api_addr", cfg.APIAddr),
		slog.String("log_level", cfg.LogLevelStr),
	)

	// Initialize Database Pool
	dbCtx, dbCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbCancel()

	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	var dbPool *pgxpool.Pool
	if err != nil {
		logger.Error("failed to parse database configuration", slog.String("error", err.Error()))
		os.Exit(1)
	} else {
		dbPool, err = pgxpool.NewWithConfig(context.Background(), poolConfig)
		if err != nil {
			logger.Error("failed to create database pool", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer dbPool.Close()

		if err := dbPool.Ping(dbCtx); err != nil {
			logger.Warn("initial database ping failed, readyz will report unavailable until connected", slog.String("error", err.Error()))
		} else {
			logger.Info("connected to timescaledb/postgres successfully")
		}
	}

	// Initialize Redis Client
	redisOpt, err := redis.ParseURL(cfg.RedisURL)
	var redisClient *redis.Client
	if err != nil {
		logger.Error("failed to parse redis url", slog.String("error", err.Error()))
		os.Exit(1)
	} else {
		redisClient = redis.NewClient(redisOpt)
		defer redisClient.Close()

		redisCtx, redisCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer redisCancel()

		if err := redisClient.Ping(redisCtx).Err(); err != nil {
			logger.Warn("initial redis ping failed, readyz will report unavailable until connected", slog.String("error", err.Error()))
		} else {
			logger.Info("connected to redis successfully")
		}
	}

	checker := &liveReadinessChecker{
		dbPool:      dbPool,
		redisClient: redisClient,
	}

	router := httpx.NewRouter(logger, checker)

	server := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening on " + cfg.APIAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	shutdownSig := make(chan os.Signal, 1)
	signal.Notify(shutdownSig, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		logger.Error("server listener error", slog.String("error", err.Error()))
		os.Exit(1)
	case sig := <-shutdownSig:
		logger.Info(fmt.Sprintf("received shutdown signal: %v", sig))
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful server shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("server exited cleanly")
}
