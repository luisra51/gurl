package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Crawler settings
	MaxDepth           int  `json:"max_depth"`
	DeduplicateEmails  bool `json:"deduplicate_emails"`

	// Cache settings
	CacheEnabled        bool          `json:"cache_enabled"`
	CacheExpirationTime time.Duration `json:"cache_expiration_time"`

	// Async processing settings
	AsyncEnabled         bool          `json:"async_enabled"`
	AsyncWorkers         int           `json:"async_workers"`
	AsyncQueueSize       int           `json:"async_queue_size"`
	AsyncJobTimeout      time.Duration `json:"async_job_timeout"`
	AsyncWebhookTimeout  time.Duration `json:"async_webhook_timeout"`
	AsyncWebhookRetries  int           `json:"async_webhook_retries"`

	// Redis settings
	RedisHost        string `json:"redis_host"`
	RedisPort        string `json:"redis_port"`
	RedisPassword    string `json:"redis_password"`
	RedisDB          int    `json:"redis_db"`
	RedisPersistDisk bool   `json:"redis_persist_disk"`

	// Redis persistence
	RedisSaveFrequency int    `json:"redis_save_frequency"`
	RedisAOFEnabled    bool   `json:"redis_aof_enabled"`
	RedisMaxMemory     string `json:"redis_max_memory"`

	// Server settings
	ServerPort string `json:"server_port"`
	ServerHost string `json:"server_host"`
}

func Load() *Config {
	return &Config{
		// Crawler settings
		MaxDepth:          getEnvAsInt("CRAWLER_MAX_DEPTH", 3),
		DeduplicateEmails: getEnvAsBool("CRAWLER_DEDUPLICATE_EMAILS", true),

		// Cache settings
		CacheEnabled:        getEnvAsBool("CACHE_ENABLED", true),
		CacheExpirationTime: time.Duration(getEnvAsInt("CACHE_EXPIRATION_MONTHS", 12)) * 24 * 30 * time.Hour,

		// Async processing settings
		AsyncEnabled:        getEnvAsBool("ASYNC_ENABLED", true),
		AsyncWorkers:        getEnvAsInt("ASYNC_WORKERS", 3),
		AsyncQueueSize:      getEnvAsInt("ASYNC_QUEUE_SIZE", 100),
		AsyncJobTimeout:     time.Duration(getEnvAsInt("ASYNC_JOB_TIMEOUT_SECONDS", 300)) * time.Second,
		AsyncWebhookTimeout: time.Duration(getEnvAsInt("ASYNC_WEBHOOK_TIMEOUT_SECONDS", 10)) * time.Second,
		AsyncWebhookRetries: getEnvAsInt("ASYNC_WEBHOOK_RETRIES", 3),

		// Redis settings
		RedisHost:        getEnv("REDIS_HOST", "localhost"),
		RedisPort:        getEnv("REDIS_PORT", "6379"),
		RedisPassword:    getEnv("REDIS_PASSWORD", ""),
		RedisDB:          getEnvAsInt("REDIS_DB", 0),
		RedisPersistDisk: getEnvAsBool("REDIS_PERSIST_DISK", false),

		// Redis persistence
		RedisSaveFrequency: getEnvAsInt("REDIS_SAVE_FREQUENCY", 300),
		RedisAOFEnabled:    getEnvAsBool("REDIS_AOF_ENABLED", true),
		RedisMaxMemory:     getEnv("REDIS_MAX_MEMORY", "256mb"),

		// Server settings
		ServerPort: getEnv("SERVER_PORT", "8080"),
		ServerHost: getEnv("SERVER_HOST", "0.0.0.0"),
	}
}

func (c *Config) RedisAddress() string {
	return c.RedisHost + ":" + c.RedisPort
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}