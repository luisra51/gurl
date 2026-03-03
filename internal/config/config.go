package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Crawler settings
	MaxDepth           int   `json:"max_depth"`
	DeduplicateEmails  bool  `json:"deduplicate_emails"`
	MaxResponseBytes   int64 `json:"max_response_bytes"`
	MaxPagesVisited    int   `json:"max_pages_visited"`

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
	ServerPort              string        `json:"server_port"`
	ServerHost              string        `json:"server_host"`
	ServerReadHeaderTimeout time.Duration `json:"server_read_header_timeout"`
	ServerReadTimeout       time.Duration `json:"server_read_timeout"`
	ServerWriteTimeout      time.Duration `json:"server_write_timeout"`
	ServerIdleTimeout       time.Duration `json:"server_idle_timeout"`
}

func Load() *Config {
	return &Config{
		// Crawler settings
		MaxDepth:          getEnvAsInt("CRAWLER_MAX_DEPTH", 3),
		DeduplicateEmails: getEnvAsBool("CRAWLER_DEDUPLICATE_EMAILS", true),
		MaxResponseBytes:  getEnvAsInt64("CRAWLER_MAX_RESPONSE_BYTES", 1024*1024),
		MaxPagesVisited:   getEnvAsInt("CRAWLER_MAX_PAGES_VISITED", 50),

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
		ServerPort:              getEnv("SERVER_PORT", "8080"),
		ServerHost:              getEnv("SERVER_HOST", "0.0.0.0"),
		ServerReadHeaderTimeout: time.Duration(getEnvAsInt("SERVER_READ_HEADER_TIMEOUT_SECONDS", 5)) * time.Second,
		ServerReadTimeout:       time.Duration(getEnvAsInt("SERVER_READ_TIMEOUT_SECONDS", 15)) * time.Second,
		ServerWriteTimeout:      time.Duration(getEnvAsInt("SERVER_WRITE_TIMEOUT_SECONDS", 30)) * time.Second,
		ServerIdleTimeout:       time.Duration(getEnvAsInt("SERVER_IDLE_TIMEOUT_SECONDS", 60)) * time.Second,
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

func getEnvAsInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
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
