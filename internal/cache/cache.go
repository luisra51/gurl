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
	"email-crawler/internal/crawler"
)

type CachedResult struct {
	Emails         []string                `json:"emails"`
	SocialProfiles []crawler.SocialProfile `json:"social_profiles,omitempty"`
	Phones         []crawler.Phone         `json:"phones,omitempty"`
	Organizations  []crawler.Organization  `json:"organizations,omitempty"`
	Timestamp      time.Time               `json:"timestamp"`
	CrawlInfo struct {
		Depth        int `json:"depth"`
		PagesVisited int `json:"pages_visited"`
	} `json:"crawl_info"`
}

// redisOpTimeout caps individual Redis round trips so a hung Redis cannot
// stall the crawler's cache lookups.
const redisOpTimeout = 5 * time.Second

type CacheManager struct {
	client    *redis.Client
	config    *config.Config
	ctx       context.Context
	enabled   bool
	testData   map[string]*CachedResult
}

// opCtx returns a short-lived context for a single Redis operation, derived
// from the CacheManager's base context so it still honors global cancellation.
func (cm *CacheManager) opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(cm.ctx, redisOpTimeout)
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
	if cm != nil && cm.testData != nil {
		result, ok := cm.testData[rawURL]
		return result, ok
	}
	if !cm.enabled {
		return nil, false
	}

	key := cm.generateKey(rawURL)

	ctx, cancel := cm.opCtx()
	defer cancel()
	data, err := cm.client.Get(ctx, key).Result()
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

func (cm *CacheManager) Set(rawURL string, emails []string, socialProfiles []crawler.SocialProfile, phones []crawler.Phone, organizations []crawler.Organization, depth int, pagesVisited int) error {
	if cm != nil && cm.testData != nil {
		cm.testData[rawURL] = &CachedResult{
			Emails:         cm.DeduplicateEmails(emails),
			SocialProfiles: socialProfiles,
			Phones:         phones,
			Organizations:  organizations,
			Timestamp:      time.Now(),
			CrawlInfo: struct {
				Depth        int `json:"depth"`
				PagesVisited int `json:"pages_visited"`
			}{Depth: depth, PagesVisited: pagesVisited},
		}
		return nil
	}
	if !cm.enabled {
		return nil
	}

	// Deduplicate and sort emails
	deduplicatedEmails := cm.DeduplicateEmails(emails)

	result := CachedResult{
		Emails:         deduplicatedEmails,
		SocialProfiles: socialProfiles,
		Phones:         phones,
		Organizations:  organizations,
		Timestamp:      time.Now(),
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

	ctx, cancel := cm.opCtx()
	defer cancel()
	err = cm.client.Set(ctx, key, data, cm.config.CacheExpirationTime).Err()
	if err != nil {
		return fmt.Errorf("failed to set cache: %v", err)
	}

	log.Printf("Cached result for %s with %d emails", rawURL, len(deduplicatedEmails))
	return nil
}

func (cm *CacheManager) SetInMemoryResultForTest(rawURL string, result *CachedResult) {
	if cm.testData == nil {
		cm.testData = make(map[string]*CachedResult)
	}
	cm.testData[rawURL] = result
}

func (cm *CacheManager) DeduplicateEmails(emails []string) []string {
	if cm == nil || cm.config == nil {
		return emails
	}

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
	ctx, cancel := cm.opCtx()
	defer cancel()
	return cm.client.Del(ctx, key).Err()
}

func (cm *CacheManager) ClearAll() error {
	if !cm.enabled {
		return nil
	}

	// Get all keys matching our pattern
	keysCtx, keysCancel := cm.opCtx()
	keys, err := cm.client.Keys(keysCtx, "crawler:emails:*").Result()
	keysCancel()
	if err != nil {
		return err
	}

	if len(keys) > 0 {
		delCtx, delCancel := cm.opCtx()
		defer delCancel()
		return cm.client.Del(delCtx, keys...).Err()
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
	infoCtx, infoCancel := cm.opCtx()
	info, err := cm.client.Info(infoCtx, "memory").Result()
	infoCancel()
	if err == nil {
		stats["redis_info"] = info
	}

	// Count our keys
	keysCtx, keysCancel := cm.opCtx()
	keys, err := cm.client.Keys(keysCtx, "crawler:emails:*").Result()
	keysCancel()
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
