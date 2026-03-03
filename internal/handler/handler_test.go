package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"email-crawler/internal/cache"
	"email-crawler/internal/config"
	"email-crawler/internal/crawler"
)

func TestScanHandlerRejectsNonHTTPSchemes(t *testing.T) {
	h := NewHandler(&config.Config{MaxDepth: 1}, &cache.CacheManager{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/scan?url=ftp://example.com", nil)
	rec := httptest.NewRecorder()

	h.ScanHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAsyncScanHandlerRejectsNonHTTPTargetURL(t *testing.T) {
	h := NewHandler(&config.Config{AsyncEnabled: true}, &cache.CacheManager{}, nil)
	body := marshalBody(t, map[string]string{
		"url":         "mailto:test@example.com",
		"webhook_url": "https://callback.example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/scan/async", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.AsyncScanHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestAsyncScanHandlerRejectsNonHTTPWebhookURL(t *testing.T) {
	h := NewHandler(&config.Config{AsyncEnabled: true}, &cache.CacheManager{}, nil)
	body := marshalBody(t, map[string]string{
		"url":         "https://example.com",
		"webhook_url": "mailto:test@example.com",
	})
	req := httptest.NewRequest(http.MethodPost, "/scan/async", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	h.AsyncScanHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestScanHandlerIncludesSocialProfilesFromCache(t *testing.T) {
	cfg := &config.Config{MaxDepth: 1, CacheEnabled: false}
	cacheManager := &cache.CacheManager{}
	cacheManager = seedCacheManagerWithResult(t, cacheManager, "https://example.com", []string{"info@example.com"}, []crawler.SocialProfile{
		{
			Platform:   "instagram",
			URL:        "https://www.instagram.com/example",
			Handle:     "example",
			SourcePage: "https://example.com",
			Confidence: "high",
		},
	})

	h := NewHandler(cfg, cacheManager, nil)
	req := httptest.NewRequest(http.MethodGet, "/scan?url=example.com", nil)
	rec := httptest.NewRecorder()

	h.ScanHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	profiles, ok := response["social_profiles"].([]any)
	if !ok || len(profiles) != 1 {
		t.Fatalf("expected 1 social profile in response, got %#v", response["social_profiles"])
	}
}

func seedCacheManagerWithResult(t *testing.T, cm *cache.CacheManager, rawURL string, emails []string, profiles []crawler.SocialProfile) *cache.CacheManager {
	t.Helper()

	cm.SetInMemoryResultForTest(rawURL, &cache.CachedResult{
		Emails:         emails,
		SocialProfiles: profiles,
		CrawlInfo: struct {
			Depth        int `json:"depth"`
			PagesVisited int `json:"pages_visited"`
		}{Depth: 1, PagesVisited: 1},
	})

	return cm
}

func marshalBody(t *testing.T, value map[string]string) []byte {
	t.Helper()

	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	return body
}
