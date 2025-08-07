package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-redis/redis/v8"

	"email-crawler/internal/cache"
	"email-crawler/internal/config"
	"email-crawler/internal/handler"
	"email-crawler/internal/jobs"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize Redis client for both cache and jobs
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddress(),
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()

	// Initialize cache manager
	cacheManager := cache.NewCacheManager(cfg)
	defer cacheManager.Close()

	// Initialize job queue and worker pool
	var jobQueue *jobs.Queue
	var workerPool *jobs.WorkerPool

	if cfg.AsyncEnabled {
		jobQueue = jobs.NewQueue(redisClient, cfg)
		workerPool = jobs.NewWorkerPool(jobQueue, cacheManager, cfg)
		workerPool.Start()

		// Setup graceful shutdown for workers
		setupGracefulShutdown(workerPool)
	}

	// Initialize handler
	h := handler.NewHandler(cfg, cacheManager, jobQueue)

	// Setup routes
	http.HandleFunc("/scan", h.ScanHandler)
	http.HandleFunc("/cache/stats", h.CacheStatsHandler)
	http.HandleFunc("/cache/invalidate", h.InvalidateCacheHandler)

	// Async endpoints (if enabled)
	if cfg.AsyncEnabled {
		http.HandleFunc("/scan/async", h.AsyncScanHandler)
		http.HandleFunc("/scan/status/", h.JobStatusHandler)
		http.HandleFunc("/scan/cancel/", h.CancelJobHandler)
		http.HandleFunc("/scan/jobs", h.JobsListHandler)
	}

	address := cfg.ServerHost + ":" + cfg.ServerPort

	fmt.Printf("=== Email Crawler Service ===\n")
	fmt.Printf("Server listening on http://%s\n", address)
	fmt.Printf("Max crawl depth: %d\n", cfg.MaxDepth)
	fmt.Printf("Cache enabled: %v\n", cfg.CacheEnabled)
	fmt.Printf("Email deduplication: %v\n", cfg.DeduplicateEmails)
	fmt.Printf("Async processing: %v\n", cfg.AsyncEnabled)

	if cfg.CacheEnabled {
		fmt.Printf("Redis: %s\n", cfg.RedisAddress())
		fmt.Printf("Cache TTL: %.0f hours\n", cfg.CacheExpirationTime.Hours())
	}

	if cfg.AsyncEnabled {
		fmt.Printf("Workers: %d\n", cfg.AsyncWorkers)
		fmt.Printf("Job timeout: %s\n", cfg.AsyncJobTimeout)
		fmt.Printf("Webhook retries: %d\n", cfg.AsyncWebhookRetries)
	}

	fmt.Printf("\n=== API Endpoints ===\n")
	fmt.Printf("GET    /scan?url=<website>   - Scan website for emails (sync)\n")
	fmt.Printf("GET    /cache/stats          - View cache statistics\n")
	fmt.Printf("DELETE /cache/invalidate     - Clear all cache\n")
	fmt.Printf("DELETE /cache/invalidate?url=<website> - Clear specific URL cache\n")

	if cfg.AsyncEnabled {
		fmt.Printf("\n=== Async Endpoints ===\n")
		fmt.Printf("POST   /scan/async          - Queue async scan job\n")
		fmt.Printf("GET    /scan/status/<id>    - Check job status\n")
		fmt.Printf("DELETE /scan/cancel/<id>    - Cancel queued job\n")
		fmt.Printf("GET    /scan/jobs           - List active jobs\n")
	}

	fmt.Printf("\n=== Examples ===\n")
	fmt.Printf("Sync:  curl 'http://localhost:%s/scan?url=example.com'\n", cfg.ServerPort)

	if cfg.AsyncEnabled {
		fmt.Printf("Async: curl -X POST 'http://localhost:%s/scan/async' \\\n", cfg.ServerPort)
		fmt.Printf("       -H 'Content-Type: application/json' \\\n")
		fmt.Printf("       -d '{\"url\":\"example.com\",\"webhook_url\":\"https://your-api.com/webhook\"}'\n")
	}

	fmt.Printf("=============================\n\n")

	log.Fatal(http.ListenAndServe(address, nil))
}

func setupGracefulShutdown(workerPool *jobs.WorkerPool) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("Received shutdown signal...")
		if workerPool != nil {
			workerPool.Stop()
		}
		os.Exit(0)
	}()
}