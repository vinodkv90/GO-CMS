package jobs

import (
	"errors"
	"sync"
	"time"
)

// ErrQueueEmpty is returned when no jobs are currently ready to be processed.
var ErrQueueEmpty = errors.New("jobs: no ready jobs in queue")

// Queue stores and dispenses background jobs.
type Queue struct {
	mu    sync.RWMutex
	jobs  map[string]*Job
	order []string // preserves order of job IDs
}

// NewQueue creates a new in-memory job queue.
func NewQueue() *Queue {
	return &Queue{
		jobs:  make(map[string]*Job),
		order: make([]string, 0),
	}
}

// Enqueue adds a job to the queue.
func (q *Queue) Enqueue(job *Job) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.jobs[job.ID]; !exists {
		q.order = append(q.order, job.ID)
	}
	job.Status = StatusPending
	q.jobs[job.ID] = job
	return nil
}

// Dequeue retrieves the next pending job that is ready to run (RunAt <= now).
func (q *Queue) Dequeue() (*Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	now := time.Now().UTC()
	for _, id := range q.order {
		job, ok := q.jobs[id]
		if !ok {
			continue
		}
		if job.Status == StatusPending && !job.RunAt.After(now) {
			job.Status = StatusRunning
			return job, nil
		}
	}

	return nil, ErrQueueEmpty
}

// Get retrieves a job by its unique ID.
func (q *Queue) Get(id string) (*Job, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	job, ok := q.jobs[id]
	if !ok {
		return nil, false
	}
	// Return copy
	jobCopy := *job
	return &jobCopy, true
}

// Update updates the status and metadata of a job.
func (q *Queue) Update(job *Job) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.jobs[job.ID] = job
}

// List returns jobs sorted by creation time with pagination.
func (q *Queue) List(limit, offset int) ([]*Job, int) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	total := len(q.order)
	if offset >= total {
		return []*Job{}, total
	}

	end := offset + limit
	if end > total || limit <= 0 {
		end = total
	}

	res := make([]*Job, 0, end-offset)
	for i := offset; i < end; i++ {
		id := q.order[i]
		if job, ok := q.jobs[id]; ok {
			jobCopy := *job
			res = append(res, &jobCopy)
		}
	}

	return res, total
}

// Stats returns a summary of job statuses in the queue.
func (q *Queue) Stats() map[string]int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := map[string]int{
		"total":     len(q.jobs),
		"pending":   0,
		"running":   0,
		"completed": 0,
		"failed":    0,
	}

	for _, job := range q.jobs {
		stats[string(job.Status)]++
	}

	return stats
}
