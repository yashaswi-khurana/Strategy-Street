package config_test

import (
	"log/slog"
	"testing"

	"github.com/strategy-street/api/internal/config"
)

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		wantErr     bool
		errContains string
		check       func(t *testing.T, cfg *config.Config)
	}{
		{
			name: "valid minimal configuration with defaults",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@localhost:5432/mydb?sslmode=disable",
				"REDIS_URL":    "redis://localhost:6379/0",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.APIAddr != "0.0.0.0:8080" {
					t.Errorf("expected default APIAddr 0.0.0.0:8080, got %q", cfg.APIAddr)
				}
				if cfg.LogLevel != slog.LevelInfo {
					t.Errorf("expected default LogLevel LevelInfo, got %v", cfg.LogLevel)
				}
				if cfg.Env != "local" {
					t.Errorf("expected default Env 'local', got %q", cfg.Env)
				}
			},
		},
		{
			name: "valid explicit configuration",
			env: map[string]string{
				"DATABASE_URL": "postgresql://postgres:secret@127.0.0.1:5432/testdb",
				"REDIS_URL":    "rediss://127.0.0.1:6380/1",
				"API_ADDR":     "127.0.0.1:9090",
				"LOG_LEVEL":    "debug",
				"ENV":          "production",
			},
			wantErr: false,
			check: func(t *testing.T, cfg *config.Config) {
				if cfg.APIAddr != "127.0.0.1:9090" {
					t.Errorf("expected APIAddr 127.0.0.1:9090, got %q", cfg.APIAddr)
				}
				if cfg.LogLevel != slog.LevelDebug {
					t.Errorf("expected LogLevel LevelDebug, got %v", cfg.LogLevel)
				}
				if cfg.Env != "production" {
					t.Errorf("expected Env 'production', got %q", cfg.Env)
				}
			},
		},
		{
			name: "missing DATABASE_URL",
			env: map[string]string{
				"REDIS_URL": "redis://localhost:6379",
			},
			wantErr:     true,
			errContains: "missing required environment variable: DATABASE_URL",
		},
		{
			name: "missing REDIS_URL",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@localhost:5432/mydb",
			},
			wantErr:     true,
			errContains: "missing required environment variable: REDIS_URL",
		},
		{
			name: "malformed DATABASE_URL - invalid scheme",
			env: map[string]string{
				"DATABASE_URL": "mysql://user:pass@localhost:3306/mydb",
				"REDIS_URL":    "redis://localhost:6379",
			},
			wantErr:     true,
			errContains: "malformed DATABASE_URL",
		},
		{
			name: "malformed DATABASE_URL - missing host",
			env: map[string]string{
				"DATABASE_URL": "postgres:///mydb",
				"REDIS_URL":    "redis://localhost:6379",
			},
			wantErr:     true,
			errContains: "malformed DATABASE_URL",
		},
		{
			name: "malformed DATABASE_URL - invalid port",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@localhost:99999999/mydb",
				"REDIS_URL":    "redis://localhost:6379",
			},
			wantErr:     true,
			errContains: "malformed DATABASE_URL",
		},
		{
			name: "malformed REDIS_URL - invalid scheme",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@localhost:5432/mydb",
				"REDIS_URL":    "http://localhost:6379",
			},
			wantErr:     true,
			errContains: "malformed REDIS_URL",
		},
		{
			name: "malformed REDIS_URL - invalid port",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@localhost:5432/mydb",
				"REDIS_URL":    "redis://localhost:99999999/0",
			},
			wantErr:     true,
			errContains: "malformed REDIS_URL",
		},
		{
			name: "invalid LOG_LEVEL",
			env: map[string]string{
				"DATABASE_URL": "postgres://user:pass@localhost:5432/mydb",
				"REDIS_URL":    "redis://localhost:6379",
				"LOG_LEVEL":    "nonexistent",
			},
			wantErr:     true,
			errContains: "invalid LOG_LEVEL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(key string) string {
				return tt.env[key]
			}
			cfg, err := config.LoadFromEnv(lookup)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
