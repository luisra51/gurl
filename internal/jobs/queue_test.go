package jobs

import (
	"testing"
	"time"
)

func TestRecoverInterruptedJobsRequeuesProcessingJobs(t *testing.T) {
	queue := newTestQueue(t)
	processing := &ScanJob{
		ID:        "job-processing-recover",
		URL:       "https://example.com",
		Status:    StatusProcessing,
		CreatedAt: time.Now(),
	}

	if err := queue.UpdateJob(processing); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	if err := queue.client.SAdd(queue.ctx, ActiveJobsKey, processing.ID).Err(); err != nil {
		t.Fatalf("seed active job: %v", err)
	}

	requeued, err := queue.RecoverInterruptedJobs()
	if err != nil {
		t.Fatalf("recover interrupted jobs: %v", err)
	}

	if requeued != 1 {
		t.Fatalf("expected 1 requeued job, got %d", requeued)
	}

	stored, err := queue.GetJob(processing.ID)
	if err != nil {
		t.Fatalf("load job: %v", err)
	}

	if stored.Status != StatusQueued {
		t.Fatalf("expected queued status, got %s", stored.Status)
	}

	size, err := queue.GetQueueSize()
	if err != nil {
		t.Fatalf("get queue size: %v", err)
	}

	if size != 1 {
		t.Fatalf("expected queue size 1, got %d", size)
	}
}
