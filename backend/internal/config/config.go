package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr                string
	DatabasePath              string
	StaticRoot                string
	UserDataRoot              string
	EnableDebugAPI            bool
	AdminUsername             string
	AdminPassword             string
	WorkerCount               int
	PollInterval              time.Duration
	ShutdownTimeout           time.Duration
	CORSAllowedOrigins        []string
	SessionCookieSecure       string
	TrustedProxyCIDRs         []string
	HTTPReadHeaderTimeout     time.Duration
	HTTPIdleTimeout           time.Duration
	HTTPRequestTimeout        time.Duration
	LoginRatePerMinute        int
	LoginRateBurst            int
	RegisterRatePerHour       int
	UploadRatePerMinute       int
	AgentMaxConcurrentGlobal  int
	AgentMaxConcurrentPerUser int
	AgentMaxQueuedGlobal      int
	AgentMaxQueuedPerUser     int
}

func Load() Config {
	return Config{
		ListenAddr:                envOrDefault("BACKEND_ADDR", ":8080"),
		DatabasePath:              envOrDefault("BACKEND_DB_PATH", "./data/app.db"),
		StaticRoot:                envOrDefault("FRONTEND_DIST", "../dist"),
		UserDataRoot:              envOrDefault("USER_DATA_ROOT", "./data/users"),
		EnableDebugAPI:            envBoolOrDefault("ENABLE_DEBUG_API", false),
		AdminUsername:             os.Getenv("PERSEUS_ADMIN_USERNAME"),
		AdminPassword:             os.Getenv("PERSEUS_ADMIN_PASSWORD"),
		WorkerCount:               envIntOrDefault("TASK_WORKER_COUNT", 2),
		PollInterval:              envDurationOrDefault("TASK_POLL_INTERVAL", 1500*time.Millisecond),
		ShutdownTimeout:           envDurationOrDefault("BACKEND_SHUTDOWN_TIMEOUT", 10*time.Second),
		CORSAllowedOrigins:        envCSV("CORS_ALLOWED_ORIGINS"),
		SessionCookieSecure:       envEnumOrDefault("SESSION_COOKIE_SECURE", "auto", "auto", "true", "false"),
		TrustedProxyCIDRs:         envCSV("TRUSTED_PROXY_CIDRS"),
		HTTPReadHeaderTimeout:     envDurationOrDefault("HTTP_READ_HEADER_TIMEOUT", 10*time.Second),
		HTTPIdleTimeout:           envDurationOrDefault("HTTP_IDLE_TIMEOUT", 120*time.Second),
		HTTPRequestTimeout:        envDurationOrDefault("HTTP_REQUEST_TIMEOUT", 30*time.Second),
		LoginRatePerMinute:        envIntOrDefault("AUTH_LOGIN_RATE_PER_MINUTE", 5),
		LoginRateBurst:            envIntOrDefault("AUTH_LOGIN_RATE_BURST", 10),
		RegisterRatePerHour:       envIntOrDefault("AUTH_REGISTER_RATE_PER_HOUR", 3),
		UploadRatePerMinute:       envIntOrDefault("UPLOAD_RATE_PER_MINUTE", 30),
		AgentMaxConcurrentGlobal:  envIntOrDefault("AGENT_MAX_CONCURRENT_GLOBAL", 4),
		AgentMaxConcurrentPerUser: envIntOrDefault("AGENT_MAX_CONCURRENT_PER_USER", 2),
		AgentMaxQueuedGlobal:      envIntOrDefault("AGENT_MAX_QUEUED_GLOBAL", 100),
		AgentMaxQueuedPerUser:     envIntOrDefault("AGENT_MAX_QUEUED_PER_USER", 10),
	}
}

func envCSV(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	seen := map[string]bool{}
	values := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		values = append(values, value)
	}
	return values
}

func envEnumOrDefault(key, fallback string, allowed ...string) string {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	for _, item := range allowed {
		if value == item {
			return value
		}
	}
	return fallback
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
