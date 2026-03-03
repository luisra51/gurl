package jobs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"email-crawler/internal/cache"
	"email-crawler/internal/config"
	"email-crawler/internal/crawler"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
)

func TestCancelJobCancelsProcessingJobAndMarksItCancelled(t *testing.T) {
	queue := newTestQueue(t)

	job := &ScanJob{
		ID:        "job-processing",
		URL:       "https://example.com",
		Status:    StatusProcessing,
		CreatedAt: time.Now(),
	}

	if err := queue.UpdateJob(job); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	cancelled := make(chan string, 1)
	queue.SetJobCanceller(func(jobID string) bool {
		cancelled <- jobID
		return true
	})

	if err := queue.CancelJob(job.ID); err != nil {
		t.Fatalf("cancel job: %v", err)
	}

	select {
	case got := <-cancelled:
		if got != job.ID {
			t.Fatalf("expected cancel hook for %s, got %s", job.ID, got)
		}
	default:
		t.Fatalf("expected cancel hook to be called")
	}

	stored, err := queue.GetJob(job.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}

	if stored.Status != StatusCancelled {
		t.Fatalf("expected cancelled status, got %s", stored.Status)
	}

	if stored.CompletedAt == nil {
		t.Fatalf("expected completed timestamp to be set")
	}
}

func TestProcessJobFailsTimedOutCrawlPromptly(t *testing.T) {
	queue := newTestQueue(t)
	cfg := &config.Config{
		MaxDepth:             1,
		DeduplicateEmails:    true,
		CacheEnabled:         false,
		AsyncJobTimeout:      50 * time.Millisecond,
		AsyncWebhookRetries:  1,
		AsyncWebhookTimeout:  50 * time.Millisecond,
	}

	wp := NewWorkerPool(queue, &cache.CacheManager{}, cfg)
	job := &ScanJob{
		ID:        "job-timeout",
		URL:       blockingURL(t),
		Status:    StatusProcessing,
		CreatedAt: time.Now(),
	}

	if err := queue.UpdateJob(job); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	done := make(chan struct{})
	go func() {
		wp.processJob(1, job)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected processJob to stop after timeout")
	}

	stored, err := queue.GetJob(job.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}

	if stored.Status != StatusFailed {
		t.Fatalf("expected failed status after timeout, got %s", stored.Status)
	}

	if stored.Error != "Job timed out" {
		t.Fatalf("expected timeout error, got %q", stored.Error)
	}
}

func TestWebhookPayloadIncludesSocialProfiles(t *testing.T) {
	var payload WebhookPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	queue := newTestQueue(t)
	cfg := &config.Config{
		AsyncWebhookRetries: 1,
		AsyncWebhookTimeout: 100 * time.Millisecond,
	}
	wp := NewWorkerPool(queue, &cache.CacheManager{}, cfg)
	job := &ScanJob{
		ID:         "job-webhook",
		URL:        "https://example.com",
		WebhookURL: server.URL,
		Status:     StatusCompleted,
		SocialProfiles: []crawler.SocialProfile{
			{
				Platform:   "instagram",
				URL:        "https://www.instagram.com/example",
				Handle:     "example",
				SourcePage: "https://example.com",
				Confidence: "high",
			},
		},
	}

	wp.sendWebhook(1, job)

	if len(payload.SocialProfiles) != 1 {
		t.Fatalf("expected webhook social profiles, got %#v", payload.SocialProfiles)
	}
}

func newTestQueue(t *testing.T) *Queue {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return NewQueue(client, &config.Config{})
}

func blockingURL(t *testing.T) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	return server.URL
}
