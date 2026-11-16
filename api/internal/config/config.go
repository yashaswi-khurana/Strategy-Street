package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Config struct {
	DatabaseURL string
	RedisURL    string
	APIAddr     string
	LogLevel    slog.Level
	LogLevelStr string
	Env         string
}

func Load() (*Config, error) {
	return LoadFromEnv(os.Getenv)
}

func LoadFromEnv(getenv func(string) string) (*Config, error) {
	dbURL := strings.TrimSpace(getenv("DATABASE_URL"))
	if dbURL == "" {
		return nil, errors.New("missing required environment variable: DATABASE_URL")
	}

	parsedDB, err := url.Parse(dbURL)
	if err != nil || (parsedDB.Scheme != "postgres" && parsedDB.Scheme != "postgresql") || parsedDB.Host == "" {
		if err != nil {
			return nil, fmt.Errorf("malformed DATABASE_URL: %w", err)
		}
		return nil, fmt.Errorf("malformed DATABASE_URL: scheme must be postgres:// or postgresql:// with valid host")
	}

	if parsedDB.Port() != "" {
		port, err := strconv.Atoi(parsedDB.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("malformed DATABASE_URL: invalid port %q (must be 1-65535)", parsedDB.Port())
		}
	}

	if _, err := pgxpool.ParseConfig(dbURL); err != nil {
		return nil, fmt.Errorf("malformed DATABASE_URL: %w", err)
	}

	redisURL := strings.TrimSpace(getenv("REDIS_URL"))
	if redisURL == "" {
		return nil, errors.New("missing required environment variable: REDIS_URL")
	}

	parsedRedis, err := url.Parse(redisURL)
	if err != nil || (parsedRedis.Scheme != "redis" && parsedRedis.Scheme != "rediss") || parsedRedis.Host == "" {
		if err != nil {
			return nil, fmt.Errorf("malformed REDIS_URL: %w", err)
		}
		return nil, fmt.Errorf("malformed REDIS_URL: scheme must be redis:// or rediss:// with valid host")
	}

	if parsedRedis.Port() != "" {
		port, err := strconv.Atoi(parsedRedis.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("malformed REDIS_URL: invalid port %q (must be 1-65535)", parsedRedis.Port())
		}
	}

	if _, err := redis.ParseURL(redisURL); err != nil {
		return nil, fmt.Errorf("malformed REDIS_URL: %w", err)
	}

	apiAddr := strings.TrimSpace(getenv("API_ADDR"))
	if apiAddr == "" {
		apiAddr = "0.0.0.0:8080"
	}

	logLevelStr := strings.TrimSpace(getenv("LOG_LEVEL"))
	if logLevelStr == "" {
		logLevelStr = "info"
	}

	var logLevel slog.Level
	switch strings.ToLower(logLevelStr) {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn", "warning":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		return nil, fmt.Errorf("invalid LOG_LEVEL: %q (must be debug, info, warn, or error)", logLevelStr)
	}

	env := strings.TrimSpace(getenv("ENV"))
	if env == "" {
		env = "local"
	}

	return &Config{
		DatabaseURL: dbURL,
		RedisURL:    redisURL,
		APIAddr:     apiAddr,
		LogLevel:    logLevel,
		LogLevelStr: strings.ToLower(logLevelStr),
		Env:         env,
	}, nil
}
