package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// WorkerPool manages concurrent worker goroutines consuming jobs from the Queue.
type WorkerPool struct {
	queue        *Queue
	handlers     map[string]Handler
	mu           sync.RWMutex
	concurrency  int
	pollInterval time.Duration
	stopChan     chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
	isRunning    bool
}

// NewWorkerPool creates a new WorkerPool.
func NewWorkerPool(queue *Queue, concurrency int, pollInterval time.Duration, logger *slog.Logger) *WorkerPool {
	if concurrency <= 0 {
		concurrency = 2
	}
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &WorkerPool{
		queue:        queue,
		handlers:     make(map[string]Handler),
		concurrency:  concurrency,
		pollInterval: pollInterval,
		stopChan:     make(chan struct{}),
		logger:       logger,
	}
}

// RegisterHandler binds an execution handler to a job type string.
func (wp *WorkerPool) RegisterHandler(jobType string, h Handler) {
	wp.mu.Lock()
	defer wp.mu.Unlock()
	wp.handlers[jobType] = h
}

// Start spawns worker goroutines.
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	if wp.isRunning {
		wp.mu.Unlock()
		return
	}
	wp.isRunning = true
	wp.mu.Unlock()

	wp.logger.Info("Starting background jobs worker pool", "concurrency", wp.concurrency)

	for i := 0; i < wp.concurrency; i++ {
		wp.wg.Add(1)
		go wp.workerLoop(i)
	}
}

// Stop initiates graceful shutdown of all workers and waits for currently executing jobs to finish.
func (wp *WorkerPool) Stop(ctx context.Context) error {
	wp.mu.Lock()
	if !wp.isRunning {
		wp.mu.Unlock()
		return nil
	}
	wp.isRunning = false
	close(wp.stopChan)
	wp.mu.Unlock()

	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wp.logger.Info("Background worker pool stopped cleanly")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("jobs: worker pool shutdown timed out: %w", ctx.Err())
	}
}

func (wp *WorkerPool) workerLoop(workerID int) {
	defer wp.wg.Done()

	ticker := time.NewTicker(wp.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-wp.stopChan:
			return
		case <-ticker.C:
			wp.processNextJob(workerID)
		}
	}
}

func (wp *WorkerPool) processNextJob(workerID int) {
	job, err := wp.queue.Dequeue()
	if err != nil {
		// Queue empty or no ready jobs
		return
	}

	wp.mu.RLock()
	handler, exists := wp.handlers[job.Type]
	wp.mu.RUnlock()

	if !exists {
		job.Status = StatusFailed
		job.Error = fmt.Sprintf("unregistered job handler for type %q", job.Type)
		now := time.Now().UTC()
		job.CompletedAt = &now
		wp.queue.Update(job)
		wp.logger.Error("Job failed: unregistered handler", "job_id", job.ID, "type", job.Type)
		return
	}

	job.Attempts++
	wp.logger.Debug("Worker executing job", "worker_id", workerID, "job_id", job.ID, "type", job.Type, "attempt", job.Attempts)

	// Execute with timeout
	jobCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	execErr := handler(jobCtx, job)
	cancel()

	now := time.Now().UTC()
	if execErr == nil {
		job.Status = StatusCompleted
		job.CompletedAt = &now
		job.Error = ""
		wp.queue.Update(job)
		wp.logger.Info("Job completed successfully", "job_id", job.ID, "type", job.Type)
		return
	}

	// Execution failed
	job.Error = execErr.Error()
	if job.Attempts < job.MaxRetries {
		// Exponential backoff: 2^(attempts-1) * 200ms
		backoff := time.Duration(1<<(job.Attempts-1)) * 200 * time.Millisecond
		job.Status = StatusPending
		job.RunAt = now.Add(backoff)
		wp.queue.Update(job)
		wp.logger.Warn("Job failed, scheduled retry",
			"job_id", job.ID,
			"attempt", job.Attempts,
			"max_retries", job.MaxRetries,
			"retry_in", backoff,
			"error", execErr,
		)
	} else {
		// Permanently failed
		job.Status = StatusFailed
		job.CompletedAt = &now
		wp.queue.Update(job)
		wp.logger.Error("Job permanently failed (retries exhausted)",
			"job_id", job.ID,
			"type", job.Type,
			"attempts", job.Attempts,
			"error", execErr,
		)
	}
}

// Queue returns the underlying job queue.
func (wp *WorkerPool) Queue() *Queue {
	return wp.queue
}

