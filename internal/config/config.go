// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	App     AppConfig
	HTTP    HTTPConfig
	MySQL   MySQLConfig
	OpenAI  OpenAIConfig
	Predict PredictConfig
}

type AppConfig struct {
	Env             string
	LogLevel        string
	ShutdownTimeout time.Duration
	APIKey          string
	WebDir          string
}

type HTTPConfig struct {
	Port           int
	RateLimitRPS   int
	RateLimitBurst int
}

type MySQLConfig struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Timeout time.Duration
}

func (o OpenAIConfig) Enabled() bool { return o.APIKey != "" }

type PredictConfig struct {
	MonteCarloSimulations int
}

func Load() (*Config, error) {
	l := &loader{}
	cfg := &Config{
		App: AppConfig{
			Env:             getString("APP_ENV", "local"),
			LogLevel:        getString("LOG_LEVEL", "info"),
			ShutdownTimeout: l.duration("SHUTDOWN_TIMEOUT", 15*time.Second),
			APIKey:          os.Getenv("API_KEY"),
			WebDir:          webDir(),
		},
		HTTP: HTTPConfig{
			Port:           l.int("APP_PORT", 8080),
			RateLimitRPS:   l.int("RATE_LIMIT_RPS", 20),
			RateLimitBurst: l.int("RATE_LIMIT_BURST", 40),
		},
		MySQL: MySQLConfig{
			DSN:             os.Getenv("MYSQL_DSN"),
			MaxOpenConns:    l.int("MYSQL_MAX_OPEN_CONNS", 20),
			MaxIdleConns:    l.int("MYSQL_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: l.duration("MYSQL_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		OpenAI: OpenAIConfig{
			APIKey:  os.Getenv("OPENAI_API_KEY"),
			Model:   getString("OPENAI_MODEL", "gpt-4.1-mini"),
			BaseURL: getString("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			Timeout: l.duration("OPENAI_TIMEOUT", 20*time.Second),
		},
		Predict: PredictConfig{
			MonteCarloSimulations: l.int("MONTECARLO_SIMULATIONS", 10000),
		},
	}
	// Report every bad numeric/duration var at once instead of defaulting.
	if len(l.errs) > 0 {
		return nil, fmt.Errorf("config: %w", errors.Join(l.errs...))
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.MySQL.DSN == "" {
		return errors.New("MYSQL_DSN is required")
	}
	if c.HTTP.Port <= 0 || c.HTTP.Port > 65535 {
		return fmt.Errorf("APP_PORT out of range: %d", c.HTTP.Port)
	}
	// RPS == 0 is the documented "disabled" switch; negative would silently
	// disable the limiter via the rps <= 0 branch in middleware.
	if c.HTTP.RateLimitRPS < 0 {
		return fmt.Errorf("RATE_LIMIT_RPS must be >= 0 (0 disables), got %d", c.HTTP.RateLimitRPS)
	}
	if c.HTTP.RateLimitBurst < 0 {
		return fmt.Errorf("RATE_LIMIT_BURST must be >= 0, got %d", c.HTTP.RateLimitBurst)
	}
	// A non-positive shutdown timeout yields an expired context, skipping
	// graceful drain entirely.
	if c.App.ShutdownTimeout <= 0 {
		return fmt.Errorf("SHUTDOWN_TIMEOUT must be > 0, got %s", c.App.ShutdownTimeout)
	}
	if c.Predict.MonteCarloSimulations < 100 {
		return fmt.Errorf("MONTECARLO_SIMULATIONS must be >= 100, got %d", c.Predict.MonteCarloSimulations)
	}
	return nil
}

func getString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// webDir defaults to ./web when WEB_DIR is unset, WEB_DIR= disables serving.
func webDir() string {
	if v, ok := os.LookupEnv("WEB_DIR"); ok {
		return v
	}
	return "./web"
}

// loader parses numeric env vars, accumulating errors so Load can report every
// bad value in one shot instead of failing on the first.
type loader struct {
	errs []error
}

func (l *loader) int(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: invalid int %q", key, v))
		return def
	}
	return n
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("%s: invalid duration %q", key, v))
		return def
	}
	return d
}
