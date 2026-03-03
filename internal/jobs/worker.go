package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"email-crawler/internal/cache"
	"email-crawler/internal/config"
	"email-crawler/internal/crawler"
)

type WorkerPool struct {
	queue        *Queue
	cacheManager *cache.CacheManager
	config       *config.Config
	workers      []chan bool
	ctx          context.Context
	cancel       context.CancelFunc
	cancelMu     sync.Mutex
	jobCancels   map[string]context.CancelFunc
}

func NewWorkerPool(queue *Queue, cacheManager *cache.CacheManager, config *config.Config) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &WorkerPool{
		queue:        queue,
		cacheManager: cacheManager,
		config:       config,
		workers:      make([]chan bool, config.AsyncWorkers),
		ctx:          ctx,
		cancel:       cancel,
		jobCancels:   make(map[string]context.CancelFunc),
	}
}

func (wp *WorkerPool) Start() {
	log.Printf("Starting %d async workers", wp.config.AsyncWorkers)
	wp.queue.SetJobCanceller(wp.cancelJob)
	
	for i := 0; i < wp.config.AsyncWorkers; i++ {
		wp.workers[i] = make(chan bool)
		go wp.worker(i, wp.workers[i])
	}
}

func (wp *WorkerPool) Stop() {
	log.Println("Stopping worker pool...")
	wp.cancel()
	
	// Signal all workers to stop
	for i, worker := range wp.workers {
		log.Printf("Stopping worker %d", i)
		close(worker)
	}
	
	log.Println("All workers stopped")
}

func (wp *WorkerPool) worker(id int, stop chan bool) {
	log.Printf("Worker %d started", id)
	
	for {
		select {
		case <-stop:
			log.Printf("Worker %d stopping", id)
			return
		case <-wp.ctx.Done():
			log.Printf("Worker %d context cancelled", id)
			return
		default:
			// Try to dequeue a job
			job, err := wp.queue.Dequeue(5 * time.Second) // 5 second timeout
			if err != nil {
				log.Printf("Worker %d: dequeue error: %v", id, err)
				continue
			}
			
			if job == nil {
				// No jobs available, continue polling
				continue
			}
			
			log.Printf("Worker %d: processing job %s for URL: %s", id, job.ID, job.URL)
			wp.processJob(id, job)
		}
	}
}

func (wp *WorkerPool) processJob(workerID int, job *ScanJob) {
	startTime := time.Now()
	
	// Check cache first
	if cachedResult, found := wp.cacheManager.Get(job.URL); found {
		log.Printf("Worker %d: cache hit for job %s", workerID, job.ID)
		
		crawlTime := time.Since(startTime).String()
		err := wp.queue.CompleteJob(job, cachedResult.Emails, cachedResult.SocialProfiles, cachedResult.CrawlInfo.PagesVisited, crawlTime)
		if err != nil {
			log.Printf("Worker %d: failed to complete cached job %s: %v", workerID, job.ID, err)
			wp.queue.FailJob(job, fmt.Sprintf("Failed to complete job: %v", err))
			return
		}
		
		wp.sendWebhook(workerID, job)
		return
	}
	
	// Parse URL
	startURL, err := url.Parse(job.URL)
	if err != nil {
		log.Printf("Worker %d: invalid URL for job %s: %v", workerID, job.ID, err)
		wp.queue.FailJob(job, fmt.Sprintf("Invalid URL: %v", err))
		wp.sendWebhook(workerID, job)
		return
	}
	
	// Create crawler with timeout context
	crawlerCtx, crawlerCancel := context.WithTimeout(wp.ctx, wp.config.AsyncJobTimeout)
	defer crawlerCancel()
	
	// Perform crawl
	c := crawler.NewWithOptions(wp.config.MaxDepth, wp.config.MaxResponseBytes, wp.config.MaxPagesVisited)
	wp.registerJobCancel(job.ID, crawlerCancel)
	defer wp.unregisterJobCancel(job.ID)

	crawlResult := c.CrawlResults(crawlerCtx, startURL)

	if err := crawlerCtx.Err(); err != nil {
		latestJob, getErr := wp.queue.GetJob(job.ID)
		if getErr == nil {
			job = latestJob
		}

		switch {
		case errors.Is(err, context.DeadlineExceeded):
			log.Printf("Worker %d: job %s timed out", workerID, job.ID)
			if job.Status != StatusCancelled {
				if failErr := wp.queue.FailJob(job, "Job timed out"); failErr != nil {
					log.Printf("Worker %d: failed to mark timed out job %s: %v", workerID, job.ID, failErr)
				}
			}
		case errors.Is(err, context.Canceled):
			log.Printf("Worker %d: job %s cancelled", workerID, job.ID)
		}

		latestJob, getErr = wp.queue.GetJob(job.ID)
		if getErr == nil {
			job = latestJob
		}

		wp.sendWebhook(workerID, job)
		return
	}
	
	// Convert map to slice
	emailList := crawlResult.Emails
	
	// Cache the result
	wp.cacheManager.Set(job.URL, emailList, crawlResult.SocialProfiles, wp.config.MaxDepth, crawlResult.PagesVisited)
	
	// Get deduplicated emails
	deduplicatedEmails := wp.cacheManager.DeduplicateEmails(emailList)
	
	crawlTime := time.Since(startTime).String()
	
	// Complete job
	err = wp.queue.CompleteJob(job, deduplicatedEmails, crawlResult.SocialProfiles, crawlResult.PagesVisited, crawlTime)
	if err != nil {
		log.Printf("Worker %d: failed to complete job %s: %v", workerID, job.ID, err)
		wp.queue.FailJob(job, fmt.Sprintf("Failed to complete job: %v", err))
	}
	
	log.Printf("Worker %d: completed job %s in %s, found %d emails", 
		workerID, job.ID, crawlTime, len(deduplicatedEmails))
	
	// Send webhook
	wp.sendWebhook(workerID, job)
}

func (wp *WorkerPool) registerJobCancel(jobID string, cancel context.CancelFunc) {
	wp.cancelMu.Lock()
	defer wp.cancelMu.Unlock()
	wp.jobCancels[jobID] = cancel
}

func (wp *WorkerPool) unregisterJobCancel(jobID string) {
	wp.cancelMu.Lock()
	defer wp.cancelMu.Unlock()
	delete(wp.jobCancels, jobID)
}

func (wp *WorkerPool) cancelJob(jobID string) bool {
	wp.cancelMu.Lock()
	cancel, ok := wp.jobCancels[jobID]
	wp.cancelMu.Unlock()
	if !ok {
		return false
	}

	cancel()
	return true
}

func (wp *WorkerPool) sendWebhook(workerID int, job *ScanJob) {
	if job.WebhookURL == "" {
		log.Printf("Worker %d: no webhook URL for job %s", workerID, job.ID)
		return
	}
	
	payload := WebhookPayload{
		JobID:        job.ID,
		CallbackID:   job.CallbackID,
		Status:       job.Status,
		URL:          job.URL,
		Emails:       job.Emails,
		SocialProfiles: job.SocialProfiles,
		CrawlTime:    job.CrawlTime,
		PagesVisited: job.PagesVisited,
		CompletedAt:  time.Now(),
		Error:        job.Error,
	}
	
	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Worker %d: failed to marshal webhook payload for job %s: %v", workerID, job.ID, err)
		return
	}
	
	// Try webhook delivery with retries
	for attempt := 1; attempt <= wp.config.AsyncWebhookRetries; attempt++ {
		log.Printf("Worker %d: sending webhook for job %s (attempt %d/%d)", 
			workerID, job.ID, attempt, wp.config.AsyncWebhookRetries)
		
		client := &http.Client{
			Timeout: wp.config.AsyncWebhookTimeout,
		}
		
		resp, err := client.Post(job.WebhookURL, "application/json", bytes.NewBuffer(jsonData))
		if err != nil {
			log.Printf("Worker %d: webhook attempt %d failed for job %s: %v", 
				workerID, attempt, job.ID, err)
			
			if attempt == wp.config.AsyncWebhookRetries {
				log.Printf("Worker %d: all webhook attempts failed for job %s", workerID, job.ID)
				return
			}
			
			// Exponential backoff
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
			continue
		}
		
		resp.Body.Close()
		
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			log.Printf("Worker %d: webhook delivered successfully for job %s (status: %d)", 
				workerID, job.ID, resp.StatusCode)
			return
		}
		
		log.Printf("Worker %d: webhook attempt %d returned status %d for job %s", 
			workerID, attempt, resp.StatusCode, job.ID)
		
		if attempt == wp.config.AsyncWebhookRetries {
			log.Printf("Worker %d: webhook failed with status %d for job %s", 
				workerID, resp.StatusCode, job.ID)
			return
		}
		
		// Exponential backoff
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
}
