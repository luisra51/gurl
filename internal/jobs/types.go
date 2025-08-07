package jobs

import (
	"time"
)

type JobStatus string

const (
	StatusQueued     JobStatus = "queued"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
	StatusCancelled  JobStatus = "cancelled"
)

type ScanJob struct {
	ID          string    `json:"job_id"`
	URL         string    `json:"url"`
	WebhookURL  string    `json:"webhook_url"`
	CallbackID  string    `json:"callback_id,omitempty"`
	Status      JobStatus `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CrawlTime   string    `json:"crawl_time,omitempty"`
	Error       string    `json:"error,omitempty"`
	
	// Results
	Emails       []string `json:"emails,omitempty"`
	PagesVisited int      `json:"pages_visited,omitempty"`
}

type AsyncScanRequest struct {
	URL        string `json:"url" binding:"required"`
	WebhookURL string `json:"webhook_url" binding:"required"`
	CallbackID string `json:"callback_id,omitempty"`
}

type AsyncScanResponse struct {
	JobID          string `json:"job_id"`
	Status         string `json:"status"`
	EstimatedTime  string `json:"estimated_time"`
	WebhookURL     string `json:"webhook_url"`
	CheckStatusURL string `json:"check_status_url"`
}

type WebhookPayload struct {
	JobID        string    `json:"job_id"`
	CallbackID   string    `json:"callback_id,omitempty"`
	Status       JobStatus `json:"status"`
	URL          string    `json:"url"`
	Emails       []string  `json:"emails,omitempty"`
	CrawlTime    string    `json:"crawl_time,omitempty"`
	PagesVisited int       `json:"pages_visited,omitempty"`
	CompletedAt  time.Time `json:"completed_at"`
	Error        string    `json:"error,omitempty"`
}