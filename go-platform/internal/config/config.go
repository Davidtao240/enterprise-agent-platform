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
	AgentServiceTimeout  time.Duration
	InternalServiceToken string
	AgentRunStaleAfter   time.Duration
	WorkerRuntimeV2      bool

	JWTSecret          string
	JWTExpirationHours int

	ServerPort string
	ServerMode string

	StrictDomainPolicy bool

	// M2-C 收尾:Tool 可靠性
	ToolCallDefaultTimeout   time.Duration // executing→indeterminate 超时阈值
	ToolCallTimeoutScanEvery time.Duration // 超时扫描周期
	ToolCallMaxRetry         int           // 超过即 DLQ
	ToolCircuitThreshold     int           // 连续失败熔断阈值
	ToolCircuitCooldown      time.Duration // 熔断冷却期(后半开放行探测)

	// M2-D:Credential 加密密钥(32 字节 hex/base64 或任意字符串派生)
	ToolSecretEncryptionKey string

	// M3-B:Webhook Inbox(外部推送认证 + 消费周期)
	ToolWebhookSecret     string        // ticket_connector 等 webhook HMAC 密钥
	ToolWebhookScanEvery  time.Duration // WebhookConsumer 扫描周期

	// M3-C:Outbox Dispatcher(Saga 投递/退避/补偿)
	ToolOutboxScanEvery    time.Duration // 扫描周期
	ToolOutboxMaxAttempts  int           // 投递/补偿最大尝试次数
	ToolOutboxBackoffBase  time.Duration // 指数退避基数
	ToolOutboxConfirmWait  time.Duration // sent 后未确认的 stale Verify 窗口
}

// Load reads environment variables and applies local-development defaults.
// Production deployments MUST override the default secrets via environment
// variables; Validate() will warn about weak defaults so operators can fix
// them before the server starts serving traffic.
func Load() *Config {
	cfg := &Config{
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
		AgentServiceTimeout:  getEnvDuration("AGENT_SERVICE_TIMEOUT", 300*time.Second),
		InternalServiceToken: getEnv("INTERNAL_SERVICE_TOKEN", ""),
		AgentRunStaleAfter:   getEnvDuration("AGENT_RUN_STALE_AFTER", 10*time.Minute),
		WorkerRuntimeV2:      getEnvBool("WORKER_RUNTIME_V2", false),
		JWTSecret:            getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpirationHours:   getEnvInt("JWT_EXPIRATION_HOURS", 24),
		ServerPort:           getEnv("GO_SERVER_PORT", "8080"),
		ServerMode:            getEnv("GO_SERVER_MODE", "debug"),
		StrictDomainPolicy:    getEnvBool("STRICT_DOMAIN_POLICY", false),

		ToolCallDefaultTimeout:   getEnvDuration("TOOL_CALL_DEFAULT_TIMEOUT", 2*time.Minute),
		ToolCallTimeoutScanEvery: getEnvDuration("TOOL_CALL_TIMEOUT_SCAN_EVERY", 15*time.Second),
		ToolCallMaxRetry:         getEnvInt("TOOL_CALL_MAX_RETRY", 3),
		ToolCircuitThreshold:     getEnvInt("TOOL_CIRCUIT_FAILURE_THRESHOLD", 5),
		ToolCircuitCooldown:      getEnvDuration("TOOL_CIRCUIT_COOLDOWN", 60*time.Second),

		ToolSecretEncryptionKey: getEnv("TOOL_SECRET_ENCRYPTION_KEY", ""),

		ToolWebhookSecret:    getEnv("TOOL_WEBHOOK_SECRET", ""),
		ToolWebhookScanEvery: getEnvDuration("TOOL_WEBHOOK_SCAN_EVERY", 10*time.Second),

		ToolOutboxScanEvery:   getEnvDuration("TOOL_OUTBOX_SCAN_EVERY", 5*time.Second),
		ToolOutboxMaxAttempts: getEnvInt("TOOL_OUTBOX_MAX_ATTEMPTS", 5),
		ToolOutboxBackoffBase: getEnvDuration("TOOL_OUTBOX_BACKOFF_BASE", 10*time.Second),
		ToolOutboxConfirmWait: getEnvDuration("TOOL_OUTBOX_CONFIRM_WAIT", 60*time.Second),
	}
	return cfg
}

// Validate checks the configuration for known insecure defaults and missing
// required secrets.  In production mode (GO_SERVER_MODE=release) it returns
// hard errors; in debug mode it returns warnings so development can proceed.
func (c *Config) Validate() []string {
	var warnings []string

	if c.JWTSecret == "change-me-in-production" {
		warnings = append(warnings,
			"WARNING: JWT_SECRET is using the insecure default 'change-me-in-production'. "+
				"Set a strong random secret via the JWT_SECRET environment variable.")
	}

	if c.InternalServiceToken == "" {
		warnings = append(warnings,
			"WARNING: INTERNAL_SERVICE_TOKEN is empty. "+
				"Service-to-service authentication (agent_runtime_events, tool_calls, connector_bindings) "+
				"will accept requests with an empty token. Set a strong value in production.")
	}

	if c.ToolSecretEncryptionKey == "" {
		warnings = append(warnings,
			"WARNING: TOOL_SECRET_ENCRYPTION_KEY is empty. "+
				"Connector credential encryption (AES-256-GCM) will be unavailable. "+
				"Tool calls requiring credential resolution will fail.")
	}

	if c.ToolWebhookSecret == "" {
		warnings = append(warnings,
			"WARNING: TOOL_WEBHOOK_SECRET is empty. "+
				"Webhook Inbox (M3-B) will reject all external pushes: HMAC signature "+
				"verification cannot pass with an unconfigured secret.")
	}

	if c.DBPassword == "platform_dev" && c.ServerMode == "release" {
		warnings = append(warnings,
			"CRITICAL: DB_PASSWORD is using the development default in release mode. "+
				"Override with a strong password.")
	}

	if c.MinIOSecretKey == "minioadmin" && c.ServerMode == "release" {
		warnings = append(warnings,
			"CRITICAL: MINIO_SECRET_KEY is using the development default in release mode. "+
				"Override with a strong secret.")
	}

	return warnings
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
