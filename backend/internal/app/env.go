package app

import (
	"os"
	"strconv"
)

// getEnv 返回环境变量值；未配置时回退到调用方给出的默认值。
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvInt 保留原服务对非法整数配置静默回退的行为。
func getEnvInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
