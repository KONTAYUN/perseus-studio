package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr      string
	DatabasePath    string
	StaticRoot      string
	UserDataRoot    string
	EnableDebugAPI  bool
	AdminUsername   string
	AdminPassword   string
	WorkerCount     int
	PollInterval    time.Duration
	ShutdownTimeout time.Duration
}

func Load() Config {
	return Config{
		ListenAddr:      envOrDefault("BACKEND_ADDR", ":8080"),
		DatabasePath:    envOrDefault("BACKEND_DB_PATH", "./data/app.db"),
		StaticRoot:      envOrDefault("FRONTEND_DIST", "../dist"),
		UserDataRoot:    envOrDefault("USER_DATA_ROOT", "./data/users"),
		EnableDebugAPI:  envBoolOrDefault("ENABLE_DEBUG_API", false),
		AdminUsername:   os.Getenv("PERSEUS_ADMIN_USERNAME"),
		AdminPassword:   os.Getenv("PERSEUS_ADMIN_PASSWORD"),
		WorkerCount:     envIntOrDefault("TASK_WORKER_COUNT", 2),
		PollInterval:    envDurationOrDefault("TASK_POLL_INTERVAL", 1500*time.Millisecond),
		ShutdownTimeout: envDurationOrDefault("BACKEND_SHUTDOWN_TIMEOUT", 10*time.Second),
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envBoolOrDefault(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
