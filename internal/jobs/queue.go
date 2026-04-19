package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"

	"email-crawler/internal/config"
	"email-crawler/internal/crawler"
)

const (
	QueueKey      = "crawler:job_queue"
	JobKeyPrefix  = "crawler:job:"
	ActiveJobsKey = "crawler:active_jobs"

	// redisOpTimeout caps individual Redis round trips so a hung Redis
	// cannot stall workers or HTTP handlers indefinitely.
	redisOpTimeout = 5 * time.Second
)

type Queue struct {
	client       *redis.Client
	config       *config.Config
	ctx          context.Context
	mu           sync.RWMutex
	jobCanceller func(string) bool
}

func NewQueue(client *redis.Client, config *config.Config) *Queue {
	return &Queue{
		client: client,
		config: config,
		ctx:    context.Background(),
	}
}

// opCtx returns a short-lived context for a single Redis operation, derived
// from the Queue's base context so it still honors global cancellation.
func (q *Queue) opCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(q.ctx, redisOpTimeout)
}

func (q *Queue) SetJobCanceller(canceller func(string) bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobCanceller = canceller
}

func (q *Queue) Enqueue(req AsyncScanRequest) (*ScanJob, error) {
	jobID := uuid.New().String()
	
	job := &ScanJob{
		ID:         jobID,
		URL:        req.URL,
		WebhookURL: req.WebhookURL,
		CallbackID: req.CallbackID,
		Status:     StatusQueued,
		CreatedAt:  time.Now(),
	}

	// Store job details
	jobKey := JobKeyPrefix + jobID
	jobData, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal job: %v", err)
	}

	// Set job with TTL (24 hours)
	setCtx, setCancel := q.opCtx()
	err = q.client.Set(setCtx, jobKey, jobData, 24*time.Hour).Err()
	setCancel()
	if err != nil {
		return nil, fmt.Errorf("failed to store job: %v", err)
	}

	// Add to queue
	pushCtx, pushCancel := q.opCtx()
	err = q.client.LPush(pushCtx, QueueKey, jobID).Err()
	pushCancel()
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue job: %v", err)
	}

	// Add to active jobs set
	addCtx, addCancel := q.opCtx()
	err = q.client.SAdd(addCtx, ActiveJobsKey, jobID).Err()
	addCancel()
	if err != nil {
		log.Printf("Warning: failed to add job to active set: %v", err)
	}

	log.Printf("Job %s queued for URL: %s", jobID, req.URL)
	return job, nil
}

func (q *Queue) Dequeue(timeout time.Duration) (*ScanJob, error) {
	// Blocking pop from queue
	result, err := q.client.BRPop(q.ctx, timeout, QueueKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil // No jobs available
		}
		return nil, fmt.Errorf("failed to dequeue: %v", err)
	}

	if len(result) != 2 {
		return nil, fmt.Errorf("unexpected dequeue result length: %d", len(result))
	}

	jobID := result[1]
	job, err := q.GetJob(jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job %s: %v", jobID, err)
	}

	// Update status to processing
	now := time.Now()
	job.Status = StatusProcessing
	job.StartedAt = &now

	err = q.UpdateJob(job)
	if err != nil {
		log.Printf("Warning: failed to update job status: %v", err)
	}

	return job, nil
}

func (q *Queue) GetJob(jobID string) (*ScanJob, error) {
	jobKey := JobKeyPrefix + jobID
	ctx, cancel := q.opCtx()
	defer cancel()
	data, err := q.client.Get(ctx, jobKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("job not found")
		}
		return nil, fmt.Errorf("failed to get job: %v", err)
	}

	var job ScanJob
	err = json.Unmarshal([]byte(data), &job)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %v", err)
	}

	return &job, nil
}

func (q *Queue) UpdateJob(job *ScanJob) error {
	jobKey := JobKeyPrefix + job.ID
	jobData, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %v", err)
	}

	// Update with TTL (24 hours)
	ctx, cancel := q.opCtx()
	defer cancel()
	err = q.client.Set(ctx, jobKey, jobData, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to update job: %v", err)
	}

	return nil
}

func (q *Queue) CompleteJob(job *ScanJob, emails []string, socialProfiles []crawler.SocialProfile, phones []crawler.Phone, organizations []crawler.Organization, pagesVisited int, crawlTime string) error {
	now := time.Now()
	job.Status = StatusCompleted
	job.CompletedAt = &now
	job.Emails = emails
	job.SocialProfiles = socialProfiles
	job.Phones = phones
	job.Organizations = organizations
	job.PagesVisited = pagesVisited
	job.CrawlTime = crawlTime

	err := q.UpdateJob(job)
	if err != nil {
		return err
	}

	// Remove from active jobs; log but do not fail the operation if cleanup fails.
	sremCtx, sremCancel := q.opCtx()
	if err := q.client.SRem(sremCtx, ActiveJobsKey, job.ID).Err(); err != nil {
		log.Printf("Warning: failed to remove completed job %s from active set: %v", job.ID, err)
	}
	sremCancel()

	return nil
}

func (q *Queue) FailJob(job *ScanJob, errorMsg string) error {
	now := time.Now()
	job.Status = StatusFailed
	job.CompletedAt = &now
	job.Error = errorMsg

	err := q.UpdateJob(job)
	if err != nil {
		return err
	}

	// Remove from active jobs; log but do not fail the operation if cleanup fails.
	sremCtx, sremCancel := q.opCtx()
	if err := q.client.SRem(sremCtx, ActiveJobsKey, job.ID).Err(); err != nil {
		log.Printf("Warning: failed to remove failed job %s from active set: %v", job.ID, err)
	}
	sremCancel()

	return nil
}

func (q *Queue) CancelJob(jobID string) error {
	job, err := q.GetJob(jobID)
	if err != nil {
		return err
	}

	if job.Status == StatusProcessing {
		q.mu.RLock()
		canceller := q.jobCanceller
		q.mu.RUnlock()

		if canceller == nil || !canceller(jobID) {
			return fmt.Errorf("cannot cancel job that is currently processing")
		}
	}

	now := time.Now()
	job.Status = StatusCancelled
	job.CompletedAt = &now

	err = q.UpdateJob(job)
	if err != nil {
		return err
	}

	// Remove from queue if it's still queued; log failures but keep going.
	lremCtx, lremCancel := q.opCtx()
	if err := q.client.LRem(lremCtx, QueueKey, 0, jobID).Err(); err != nil {
		log.Printf("Warning: failed to remove cancelled job %s from queue: %v", jobID, err)
	}
	lremCancel()

	sremCtx, sremCancel := q.opCtx()
	if err := q.client.SRem(sremCtx, ActiveJobsKey, jobID).Err(); err != nil {
		log.Printf("Warning: failed to remove cancelled job %s from active set: %v", jobID, err)
	}
	sremCancel()

	return nil
}

func (q *Queue) GetActiveJobs() ([]string, error) {
	ctx, cancel := q.opCtx()
	defer cancel()
	jobs, err := q.client.SMembers(ctx, ActiveJobsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active jobs: %v", err)
	}
	return jobs, nil
}

func (q *Queue) GetQueueSize() (int64, error) {
	ctx, cancel := q.opCtx()
	defer cancel()
	size, err := q.client.LLen(ctx, QueueKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get queue size: %v", err)
	}
	return size, nil
}

func (q *Queue) Stats() map[string]interface{} {
	stats := make(map[string]interface{})

	if queueSize, err := q.GetQueueSize(); err == nil {
		stats["queue_size"] = queueSize
	}

	if activeJobs, err := q.GetActiveJobs(); err == nil {
		stats["active_jobs"] = len(activeJobs)
		stats["active_job_ids"] = activeJobs
	}

	return stats
}

func (q *Queue) RecoverInterruptedJobs() (int, error) {
	activeJobs, err := q.GetActiveJobs()
	if err != nil {
		return 0, err
	}

	requeued := 0
	for _, jobID := range activeJobs {
		job, err := q.GetJob(jobID)
		if err != nil {
			continue
		}

		if job.Status != StatusProcessing {
			continue
		}

		job.Status = StatusQueued
		job.StartedAt = nil
		job.CompletedAt = nil
		job.Error = ""
		job.CrawlTime = ""
		if err := q.UpdateJob(job); err != nil {
			return requeued, err
		}

		pushCtx, pushCancel := q.opCtx()
		err = q.client.LPush(pushCtx, QueueKey, jobID).Err()
		pushCancel()
		if err != nil {
			return requeued, err
		}

		requeued++
	}

	return requeued, nil
}
