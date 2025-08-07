package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"

	"email-crawler/internal/config"
)

const (
	QueueKey      = "crawler:job_queue"
	JobKeyPrefix  = "crawler:job:"
	ActiveJobsKey = "crawler:active_jobs"
)

type Queue struct {
	client *redis.Client
	config *config.Config
	ctx    context.Context
}

func NewQueue(client *redis.Client, config *config.Config) *Queue {
	return &Queue{
		client: client,
		config: config,
		ctx:    context.Background(),
	}
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
	err = q.client.Set(q.ctx, jobKey, jobData, 24*time.Hour).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to store job: %v", err)
	}

	// Add to queue
	err = q.client.LPush(q.ctx, QueueKey, jobID).Err()
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue job: %v", err)
	}

	// Add to active jobs set
	err = q.client.SAdd(q.ctx, ActiveJobsKey, jobID).Err()
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
	data, err := q.client.Get(q.ctx, jobKey).Result()
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
	err = q.client.Set(q.ctx, jobKey, jobData, 24*time.Hour).Err()
	if err != nil {
		return fmt.Errorf("failed to update job: %v", err)
	}

	return nil
}

func (q *Queue) CompleteJob(job *ScanJob, emails []string, pagesVisited int, crawlTime string) error {
	now := time.Now()
	job.Status = StatusCompleted
	job.CompletedAt = &now
	job.Emails = emails
	job.PagesVisited = pagesVisited
	job.CrawlTime = crawlTime

	err := q.UpdateJob(job)
	if err != nil {
		return err
	}

	// Remove from active jobs
	q.client.SRem(q.ctx, ActiveJobsKey, job.ID)

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

	// Remove from active jobs
	q.client.SRem(q.ctx, ActiveJobsKey, job.ID)

	return nil
}

func (q *Queue) CancelJob(jobID string) error {
	job, err := q.GetJob(jobID)
	if err != nil {
		return err
	}

	if job.Status == StatusProcessing {
		return fmt.Errorf("cannot cancel job that is currently processing")
	}

	now := time.Now()
	job.Status = StatusCancelled
	job.CompletedAt = &now

	err = q.UpdateJob(job)
	if err != nil {
		return err
	}

	// Remove from queue if it's still queued
	q.client.LRem(q.ctx, QueueKey, 0, jobID)

	// Remove from active jobs
	q.client.SRem(q.ctx, ActiveJobsKey, jobID)

	return nil
}

func (q *Queue) GetActiveJobs() ([]string, error) {
	jobs, err := q.client.SMembers(q.ctx, ActiveJobsKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get active jobs: %v", err)
	}
	return jobs, nil
}

func (q *Queue) GetQueueSize() (int64, error) {
	size, err := q.client.LLen(q.ctx, QueueKey).Result()
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