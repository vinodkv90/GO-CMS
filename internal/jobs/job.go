package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the execution state of a background job.
type JobStatus string

const (
	StatusPending   JobStatus = "pending"
	StatusRunning   JobStatus = "running"
	StatusCompleted JobStatus = "completed"
	StatusFailed    JobStatus = "failed"
)

// Standard built-in job types across Go CMS.
const (
	TypeSendEmail        = "email:send"
	TypeWebhookDispatch  = "webhook:dispatch"
	TypeScheduledPublish = "content:publish"
	TypeMediaOptimize    = "media:optimize"
)

// Job represents a background asynchronous task.
type Job struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Status      JobStatus       `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxRetries  int             `json:"max_retries"`
	Error       string          `json:"error,omitempty"`
	RunAt       time.Time       `json:"run_at"`
	CreatedAt   time.Time       `json:"created_at"`
	CompletedAt *time.Time      `json:"completed_at,omitempty"`
}

// NewJob instantiates a new Job with default retry thresholds.
func NewJob(jobType string, payload any, runAt time.Time, maxRetries int) (*Job, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if maxRetries <= 0 {
		maxRetries = 3
	}

	now := time.Now().UTC()
	if runAt.IsZero() {
		runAt = now
	}

	return &Job{
		ID:         uuid.New().String(),
		Type:       jobType,
		Payload:    bytes,
		Status:     StatusPending,
		Attempts:   0,
		MaxRetries: maxRetries,
		RunAt:      runAt,
		CreatedAt:  now,
	}, nil
}

// Handler defines the function signature for executing a background job.
type Handler func(ctx context.Context, job *Job) error
