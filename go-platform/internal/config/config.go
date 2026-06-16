// Package config 统一管理所有环境变量配置。
// 启动时调用 config.Load() 一次性读取，后续各模块通过 cfg 字段访问。
//
// 配置来源：.env 文件（docker-compose 注入）或 OS 环境变量。
// 所有配置项都有合理默认值，本地开发无需手动设置。
package config

import (
	"os"
	"strconv"
)

// Config 聚合所有运行时配置，各模块按需取用。
type Config struct {
	// ── 数据库 ──
	DBHost     string // PostgreSQL 主机地址（默认 localhost）
	DBPort     string // PostgreSQL 端口（默认 5432）
	DBUser     string // 数据库用户名（默认 platform）
	DBPassword string // 数据库密码（默认 platform_dev）
	DBName     string // 数据库名（默认 enterprise_agent_platform）

	// ── Redis ──
	RedisHost string // Redis 主机地址（默认 localhost）
	RedisPort string // Redis 端口（默认 6379）

	// ── JWT 鉴权 ──
	JWTSecret          string // JWT 签名密钥（生产环境务必更换）
	JWTExpirationHours int    // Token 过期小时数（默认 24）

	// ── HTTP 服务 ──
	ServerPort string // Go 后端监听端口（默认 8080）
	ServerMode string // Gin 运行模式：debug / release（默认 debug）
}

// Load 从环境变量读取所有配置并返回 Config 实例。
// 未设置的变量使用默认值，保证本地开发零配置启动。
func Load() *Config {
	return &Config{
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "platform"),
		DBPassword:         getEnv("DB_PASSWORD", "platform_dev"),
		DBName:             getEnv("DB_NAME", "enterprise_agent_platform"),
		RedisHost:          getEnv("REDIS_HOST", "localhost"),
		RedisPort:          getEnv("REDIS_PORT", "6379"),
		JWTSecret:          getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpirationHours: getEnvInt("JWT_EXPIRATION_HOURS", 24),
		ServerPort:         getEnv("GO_SERVER_PORT", "8080"),
		ServerMode:         getEnv("GO_SERVER_MODE", "debug"),
	}
}

// getEnv 读取字符串环境变量，为空时返回默认值。
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt 读取整数环境变量，为空或解析失败时返回默认值。
func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
