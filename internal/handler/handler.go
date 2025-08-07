package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"email-crawler/internal/cache"
	"email-crawler/internal/config"
	"email-crawler/internal/crawler"
	"email-crawler/internal/jobs"
)

type ScanResponse struct {
	Emails     []string `json:"emails,omitempty"`
	Error      string   `json:"error,omitempty"`
	FromCache  bool     `json:"from_cache"`
	CrawlTime  string   `json:"crawl_time,omitempty"`
}

type Handler struct {
	config       *config.Config
	cacheManager *cache.CacheManager
	jobQueue     *jobs.Queue
}

func NewHandler(cfg *config.Config, cacheManager *cache.CacheManager, jobQueue *jobs.Queue) *Handler {
	return &Handler{
		config:       cfg,
		cacheManager: cacheManager,
		jobQueue:     jobQueue,
	}
}

func (h *Handler) ScanHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	w.Header().Set("Content-Type", "application/json")
	queryURL := r.URL.Query().Get("url")

	if queryURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ScanResponse{Error: "Missing 'url' parameter"})
		return
	}

	if !strings.HasPrefix(queryURL, "http://") && !strings.HasPrefix(queryURL, "https://") {
		queryURL = "https://" + queryURL
	}

	startURL, err := url.Parse(queryURL)
	if err != nil || (startURL.Scheme != "http" && startURL.Scheme != "https") {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ScanResponse{Error: "Invalid URL provided"})
		return
	}

	// Check cache first
	if cachedResult, found := h.cacheManager.Get(queryURL); found {
		crawlTime := time.Since(startTime)
		response := ScanResponse{
			Emails:    cachedResult.Emails,
			FromCache: true,
			CrawlTime: crawlTime.String(),
		}
		if len(cachedResult.Emails) == 0 {
			response.Emails = []string{} // Ensure [] instead of null
		}
		json.NewEncoder(w).Encode(response)
		return
	}

	// Not in cache, perform crawl
	c := crawler.New(h.config.MaxDepth)
	foundEmailsMap := c.Crawl(startURL)

	emailList := make([]string, 0, len(foundEmailsMap))
	for email := range foundEmailsMap {
		emailList = append(emailList, email)
	}

	// Cache the result (includes deduplication)
	h.cacheManager.Set(queryURL, emailList, h.config.MaxDepth, len(foundEmailsMap))

	// Get deduplicated emails from cache (it was just cached)
	var deduplicatedEmails []string
	if cachedResult, found := h.cacheManager.Get(queryURL); found {
		deduplicatedEmails = cachedResult.Emails
	} else {
		// Fallback - shouldn't happen but just in case
		deduplicatedEmails = h.cacheManager.DeduplicateEmails(emailList)
	}

	crawlTime := time.Since(startTime)
	response := ScanResponse{
		Emails:    deduplicatedEmails,
		FromCache: false,
		CrawlTime: crawlTime.String(),
	}
	if len(deduplicatedEmails) == 0 {
		response.Emails = []string{} // Ensure [] instead of null
	}

	json.NewEncoder(w).Encode(response)
}

// Cache management endpoints
func (h *Handler) CacheStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats := h.cacheManager.Stats()
	json.NewEncoder(w).Encode(stats)
}

func (h *Handler) InvalidateCacheHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed. Use DELETE."})
		return
	}

	queryURL := r.URL.Query().Get("url")
	if queryURL == "" {
		// Clear all cache
		if err := h.cacheManager.ClearAll(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "Failed to clear cache"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"message": "All cache cleared"})
		return
	}

	// Clear specific URL
	if err := h.cacheManager.InvalidateURL(queryURL); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to invalidate cache"})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"message": "Cache invalidated for URL", "url": queryURL})
}

// Async scan endpoints
func (h *Handler) AsyncScanHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if !h.config.AsyncEnabled {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Async scanning is disabled"})
		return
	}
	
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed. Use POST."})
		return
	}
	
	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Failed to read request body"})
		return
	}
	
	var req jobs.AsyncScanRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid JSON format"})
		return
	}
	
	// Validate required fields
	if req.URL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Missing 'url' field"})
		return
	}
	
	if req.WebhookURL == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Missing 'webhook_url' field"})
		return
	}
	
	// Validate URL format
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		req.URL = "https://" + req.URL
	}
	
	if _, err := url.Parse(req.URL); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid URL format"})
		return
	}
	
	// Validate webhook URL format
	if _, err := url.Parse(req.WebhookURL); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Invalid webhook_url format"})
		return
	}
	
	// Enqueue job
	job, err := h.jobQueue.Enqueue(req)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to queue job: %v", err)})
		return
	}
	
	// Return response
	response := jobs.AsyncScanResponse{
		JobID:          job.ID,
		Status:         string(job.Status),
		EstimatedTime:  "30-60s",
		WebhookURL:     job.WebhookURL,
		CheckStatusURL: fmt.Sprintf("/scan/status/%s", job.ID),
	}
	
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

func (h *Handler) JobStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if !h.config.AsyncEnabled {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Async scanning is disabled"})
		return
	}
	
	// Extract job ID from URL path
	// Expected path: /scan/status/{job_id}
	path := strings.TrimPrefix(r.URL.Path, "/scan/status/")
	if path == "" || path == r.URL.Path {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Missing job ID in path"})
		return
	}
	
	jobID := path
	
	// Get job from queue
	job, err := h.jobQueue.GetJob(jobID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "Job not found"})
		return
	}
	
	json.NewEncoder(w).Encode(job)
}

func (h *Handler) CancelJobHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if !h.config.AsyncEnabled {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Async scanning is disabled"})
		return
	}
	
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "Method not allowed. Use DELETE."})
		return
	}
	
	// Extract job ID from URL path
	// Expected path: /scan/cancel/{job_id}
	path := strings.TrimPrefix(r.URL.Path, "/scan/cancel/")
	if path == "" || path == r.URL.Path {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "Missing job ID in path"})
		return
	}
	
	jobID := path
	
	// Cancel job
	err := h.jobQueue.CancelJob(jobID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("Failed to cancel job: %v", err)})
		return
	}
	
	json.NewEncoder(w).Encode(map[string]string{"message": "Job cancelled", "job_id": jobID})
}

func (h *Handler) JobsListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if !h.config.AsyncEnabled {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "Async scanning is disabled"})
		return
	}
	
	// Get queue stats
	stats := h.jobQueue.Stats()
	
	response := map[string]interface{}{
		"async_enabled": h.config.AsyncEnabled,
		"queue_stats":   stats,
		"workers":       h.config.AsyncWorkers,
		"job_timeout":   h.config.AsyncJobTimeout.String(),
	}
	
	json.NewEncoder(w).Encode(response)
}