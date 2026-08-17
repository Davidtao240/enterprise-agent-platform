// Package config centralizes runtime configuration loaded from environment variables.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config groups all runtime configuration used by the backend.
type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	RedisHost string
	RedisPort string

	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	FileStorageDir string

	AgentServiceURL      string
	InternalServiceToken string
	AgentRunStaleAfter   time.Duration

	JWTSecret          string
	JWTExpirationHours int

	ServerPort string
	ServerMode string

	StrictDomainPolicy bool
}

// Load reads environment variables and applies local-development defaults.
func Load() *Config {
	return &Config{
		DBHost:               getEnv("DB_HOST", "localhost"),
		DBPort:               getEnv("DB_PORT", "5432"),
		DBUser:               getEnv("DB_USER", "platform"),
		DBPassword:           getEnv("DB_PASSWORD", "platform_dev"),
		DBName:               getEnv("DB_NAME", "enterprise_agent_platform"),
		RedisHost:            getEnv("REDIS_HOST", "localhost"),
		RedisPort:            getEnv("REDIS_PORT", "6379"),
		MinIOEndpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOAccessKey:       getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:       getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOBucket:          getEnv("MINIO_BUCKET", "platform-files"),
		FileStorageDir:       getEnv("FILE_STORAGE_DIR", "storage/files"),
		AgentServiceURL:      getEnv("AGENT_SERVICE_URL", "http://localhost:8000"),
		InternalServiceToken: getEnv("INTERNAL_SERVICE_TOKEN", ""),
		AgentRunStaleAfter:   getEnvDuration("AGENT_RUN_STALE_AFTER", 10*time.Minute),
		JWTSecret:            getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpirationHours:   getEnvInt("JWT_EXPIRATION_HOURS", 24),
		ServerPort:           getEnv("GO_SERVER_PORT", "8080"),
		ServerMode:           getEnv("GO_SERVER_MODE", "debug"),
		StrictDomainPolicy:   getEnvBool("STRICT_DOMAIN_POLICY", false),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
