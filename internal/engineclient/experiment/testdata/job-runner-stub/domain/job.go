package domain

import (
	"errors"
	"time"
)

// JobStatus represents the lifecycle state of a background job.
type JobStatus string

const (
	StatusPending JobStatus = "pending"
	StatusRunning JobStatus = "running"
	StatusDone    JobStatus = "done"
	StatusFailed  JobStatus = "failed"
)

// ErrJobNotFound is returned when a job cannot be located by ID.
var ErrJobNotFound = errors.New("job not found")

// Job is the core domain type for background work units.
type Job struct {
	ID        string
	Type      string
	Payload   []byte
	Status    JobStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}
