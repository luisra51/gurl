package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"email-crawler/internal/config"
)

type CachedResult struct {
	Emails    []string  `json:"emails"`
	Timestamp time.Time `json:"timestamp"`
	CrawlInfo struct {
		Depth        int `json:"depth"`
		PagesVisited int `json:"pages_visited"`
	} `json:"crawl_info"`
}

type CacheManager struct {
	client    *redis.Client
	config    *config.Config
	ctx       context.Context
	enabled   bool
}

func NewCacheManager(cfg *config.Config) *CacheManager {
	ctx := context.Background()
	
	if !cfg.CacheEnabled {
		log.Println("Cache is disabled")
		return &CacheManager{
			config:  cfg,
			ctx:     ctx,
			enabled: false,
		}
	}

	// Create Redis client
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddress(),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})

	// Test connection
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("Failed to connect to Redis: %v. Cache will be disabled.", err)
		return &CacheManager{
			config:  cfg,
			ctx:     ctx,
			enabled: false,
		}
	}

	log.Printf("Connected to Redis at %s", cfg.RedisAddress())

	return &CacheManager{
		client:  client,
		config:  cfg,
		ctx:     ctx,
		enabled: true,
	}
}

func (cm *CacheManager) generateKey(rawURL string) string {
	// Normalize URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Sprintf("crawler:emails:%x", sha256.Sum256([]byte(rawURL)))
	}
	
	// Create normalized URL (lowercase domain, remove trailing slash)
	normalizedURL := strings.ToLower(parsedURL.Host) + parsedURL.Path
	normalizedURL = strings.TrimSuffix(normalizedURL, "/")
	
	// Generate SHA256 hash
	hash := sha256.Sum256([]byte(normalizedURL))
	return fmt.Sprintf("crawler:emails:%x", hash)
}

func (cm *CacheManager) Get(rawURL string) (*CachedResult, bool) {
	if !cm.enabled {
		return nil, false
	}

	key := cm.generateKey(rawURL)
	
	data, err := cm.client.Get(cm.ctx, key).Result()
	if err != nil {
		if err != redis.Nil {
			log.Printf("Redis GET error: %v", err)
		}
		return nil, false
	}

	var result CachedResult
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		log.Printf("Failed to unmarshal cached result: %v", err)
		return nil, false
	}

	return &result, true
}

func (cm *CacheManager) Set(rawURL string, emails []string, depth int, pagesVisited int) error {
	if !cm.enabled {
		return nil
	}

	// Deduplicate and sort emails
	deduplicatedEmails := cm.DeduplicateEmails(emails)

	result := CachedResult{
		Emails:    deduplicatedEmails,
		Timestamp: time.Now(),
		CrawlInfo: struct {
			Depth        int `json:"depth"`
			PagesVisited int `json:"pages_visited"`
		}{
			Depth:        depth,
			PagesVisited: pagesVisited,
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %v", err)
	}

	key := cm.generateKey(rawURL)
	
	err = cm.client.Set(cm.ctx, key, data, cm.config.CacheExpirationTime).Err()
	if err != nil {
		return fmt.Errorf("failed to set cache: %v", err)
	}

	log.Printf("Cached result for %s with %d emails", rawURL, len(deduplicatedEmails))
	return nil
}

func (cm *CacheManager) DeduplicateEmails(emails []string) []string {
	if !cm.config.DeduplicateEmails {
		return emails
	}

	// Use map to remove duplicates and normalize
	emailMap := make(map[string]bool)
	
	for _, email := range emails {
		// Normalize: trim whitespace and convert to lowercase
		normalizedEmail := strings.TrimSpace(strings.ToLower(email))
		if normalizedEmail != "" {
			emailMap[normalizedEmail] = true
		}
	}

	// Convert back to slice
	deduplicatedEmails := make([]string, 0, len(emailMap))
	for email := range emailMap {
		deduplicatedEmails = append(deduplicatedEmails, email)
	}

	// Sort for consistency
	sort.Strings(deduplicatedEmails)

	return deduplicatedEmails
}

func (cm *CacheManager) InvalidateURL(rawURL string) error {
	if !cm.enabled {
		return nil
	}

	key := cm.generateKey(rawURL)
	return cm.client.Del(cm.ctx, key).Err()
}

func (cm *CacheManager) ClearAll() error {
	if !cm.enabled {
		return nil
	}

	// Get all keys matching our pattern
	keys, err := cm.client.Keys(cm.ctx, "crawler:emails:*").Result()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		return cm.client.Del(cm.ctx, keys...).Err()
	}

	return nil
}

func (cm *CacheManager) Stats() map[string]interface{} {
	stats := map[string]interface{}{
		"enabled": cm.enabled,
	}

	if !cm.enabled {
		return stats
	}

	// Get Redis info
	info, err := cm.client.Info(cm.ctx, "memory").Result()
	if err == nil {
		stats["redis_info"] = info
	}

	// Count our keys
	keys, err := cm.client.Keys(cm.ctx, "crawler:emails:*").Result()
	if err == nil {
		stats["cached_urls"] = len(keys)
	}

	return stats
}

func (cm *CacheManager) Close() error {
	if cm.enabled && cm.client != nil {
		return cm.client.Close()
	}
	return nil
}