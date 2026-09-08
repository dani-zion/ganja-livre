package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration loaded from environment variables.
// All secrets are read from env — never hardcoded.
type Config struct {
	App      AppConfig
	MongoDB  MongoConfig
	JWT      JWTConfig
	Temporal TemporalConfig
	Server   ServerConfig
}

type AppConfig struct {
	Env   string // "development" | "staging" | "production"
	Debug bool
}

type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

type MongoConfig struct {
	URI            string
	Database       string
	ConnectTimeout time.Duration
	MaxPoolSize    uint64
}

type JWTConfig struct {
	AccessSecret       string
	RefreshSecret      string
	AccessTokenExpiry  time.Duration
	RefreshTokenExpiry time.Duration
}

type TemporalConfig struct {
	HostPort  string
	Namespace string
	TaskQueue string
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		App: AppConfig{
			Env:   getEnv("APP_ENV", "development"),
			Debug: getEnvBool("APP_DEBUG", false),
		},
		Server: ServerConfig{
			Port:            getEnv("SERVER_PORT", "8080"),
			ReadTimeout:     getEnvDuration("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getEnvDuration("SERVER_WRITE_TIMEOUT", 15*time.Second),
			ShutdownTimeout: getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		MongoDB: MongoConfig{
			URI:            mustGetEnv("MONGODB_URI"),
			Database:       getEnv("MONGODB_DATABASE", "ganja-livre"),
			ConnectTimeout: getEnvDuration("MONGODB_CONNECT_TIMEOUT", 10*time.Second),
			MaxPoolSize:    getEnvUint64("MONGODB_MAX_POOL_SIZE", 100),
		},
		JWT: JWTConfig{
			AccessSecret:       mustGetEnv("JWT_ACCESS_SECRET"),
			RefreshSecret:      mustGetEnv("JWT_REFRESH_SECRET"),
			AccessTokenExpiry:  getEnvDuration("JWT_ACCESS_EXPIRY", 15*time.Minute),
			RefreshTokenExpiry: getEnvDuration("JWT_REFRESH_EXPIRY", 7*24*time.Hour),
		},
		Temporal: TemporalConfig{
			HostPort:  getEnv("TEMPORAL_HOST_PORT", "temporal:7233"),
			Namespace: getEnv("TEMPORAL_NAMESPACE", "default"),
			TaskQueue: getEnv("TEMPORAL_TASK_QUEUE", "ganja-livre-sales"),
		},
	}

	return cfg, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func mustGetEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required env var %q is not set", key))
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return d
}

func getEnvUint64(key string, fallback uint64) uint64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}
