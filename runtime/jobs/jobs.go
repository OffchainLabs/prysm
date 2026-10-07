// Package jobs provides a small in-memory registry for tracking the status and
// progress of long-running node operations, so they can be reported through the
// node API instead of only through logs.
package jobs

import (
	"fmt"
	"sync"
	"time"
)

// Status describes the lifecycle state of a job.
type Status string

const (
	// StatusPending indicates the job is registered but has not started running yet.
	StatusPending Status = "pending"
	// StatusRunning indicates the job is currently running.
	StatusRunning Status = "running"
	// StatusCompleted indicates the job finished successfully or had nothing to do.
	StatusCompleted Status = "completed"
	// StatusFailed indicates the job stopped because of an error.
	StatusFailed Status = "failed"
	// StatusCanceled indicates the job was stopped before completing, e.g. by node shutdown.
	StatusCanceled Status = "canceled"
)

// Terminal returns true when the status is final and the job will not update again.
func (s Status) Terminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCanceled
}

// Progress describes how much of a job is done, in units chosen by the producer.
type Progress struct {
	Current uint64
	Total   uint64
	Units   string
}

// Job is a point-in-time snapshot of a tracked operation.
type Job struct {
	ID         string
	Status     Status
	Phase      string
	Progress   *Progress
	Error      string
	StartedAt  time.Time // zero until the job starts running
	UpdatedAt  time.Time
	FinishedAt time.Time // zero until the job reaches a terminal status
}

// Registry tracks long-running operations. It is safe for concurrent use, and all
// methods are safe to call on a nil receiver so that wiring a registry is optional.
type Registry struct {
	mu    sync.RWMutex
	jobs  map[string]*Tracker
	order []string
}

// NewRegistry creates an empty job registry.
func NewRegistry() *Registry {
	return &Registry{jobs: make(map[string]*Tracker)}
}

// Register adds a job with the given id and returns the Tracker used to update it.
// A job in a terminal state may be registered again under the same id, representing
// a new run; registering over a pending or running job is an error. On a nil
// registry it returns a nil Tracker, which is safe to use as a no-op.
func (r *Registry) Register(id string) (*Tracker, error) {
	if r == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.jobs[id]; ok {
		if !existing.snapshot().Status.Terminal() {
			return nil, fmt.Errorf("job %q is already registered and active", id)
		}
	} else {
		r.order = append(r.order, id)
	}
	t := newTracker(id)
	r.jobs[id] = t
	return t, nil
}

// Get returns a snapshot of the job with the given id.
func (r *Registry) Get(id string) (Job, bool) {
	if r == nil {
		return Job{}, false
	}
	r.mu.RLock()
	t, ok := r.jobs[id]
	r.mu.RUnlock()
	if !ok {
		return Job{}, false
	}
	return t.snapshot(), true
}

// List returns snapshots of all jobs in registration order.
func (r *Registry) List() []Job {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Job, 0, len(r.order))
	for _, id := range r.order {
		if t, ok := r.jobs[id]; ok {
			out = append(out, t.snapshot())
		}
	}
	return out
}

// Tracker is the producer-side handle used to update a job as it runs. All methods
// are safe to call on a nil receiver and become no-ops once the job reaches a
// terminal status, so producers do not need to guard their transitions.
type Tracker struct {
	mu  sync.Mutex
	job Job
	now func() time.Time
}

func newTracker(id string) *Tracker {
	t := &Tracker{now: time.Now}
	t.job = Job{ID: id, Status: StatusPending, UpdatedAt: t.now()}
	return t
}

// Start transitions a pending job to running and records the start time.
func (t *Tracker) Start() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job.Status != StatusPending {
		return
	}
	t.job.Status = StatusRunning
	t.job.StartedAt = t.now()
	t.job.UpdatedAt = t.job.StartedAt
}

// SetPhase updates the human-readable description of what the job is currently doing.
func (t *Tracker) SetPhase(phase string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job.Status.Terminal() {
		return
	}
	t.job.Phase = phase
	t.job.UpdatedAt = t.now()
}

// SetProgress updates the completion counters of the job, in producer-defined units.
func (t *Tracker) SetProgress(current, total uint64, units string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job.Status.Terminal() {
		return
	}
	t.job.Progress = &Progress{Current: current, Total: total, Units: units}
	t.job.UpdatedAt = t.now()
}

// Complete marks the job as successfully finished.
func (t *Tracker) Complete() {
	t.finish(StatusCompleted, nil)
}

// Fail marks the job as stopped by the given error.
func (t *Tracker) Fail(err error) {
	t.finish(StatusFailed, err)
}

// Cancel marks the job as stopped before completion.
func (t *Tracker) Cancel() {
	t.finish(StatusCanceled, nil)
}

func (t *Tracker) finish(status Status, err error) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job.Status.Terminal() {
		return
	}
	t.job.Status = status
	if err != nil {
		t.job.Error = err.Error()
	}
	t.job.FinishedAt = t.now()
	t.job.UpdatedAt = t.job.FinishedAt
}

func (t *Tracker) snapshot() Job {
	t.mu.Lock()
	defer t.mu.Unlock()
	j := t.job
	if t.job.Progress != nil {
		p := *t.job.Progress
		j.Progress = &p
	}
	return j
}
