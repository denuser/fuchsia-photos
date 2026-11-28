package ml

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// State tells whether the ML service can be used.
type State string

const (
	// StateDisabled means no service is configured.
	StateDisabled State = "disabled"
	// StateUnavailable means the service does not answer.
	StateUnavailable State = "unavailable"
	// StateAvailable means the service answers with the configured model.
	StateAvailable State = "available"
	// StateMismatch means the service answers with an incompatible model.
	StateMismatch State = "mismatch"
)

// Status is the last known state of the service.
type Status struct {
	State State `json:"state"`
	// Model is the configured model.
	Model string `json:"model"`
	// ServiceModel is the model the service reported, if it answered.
	ServiceModel string     `json:"serviceModel,omitempty"`
	Device       string     `json:"device,omitempty"`
	Error        string     `json:"error,omitempty"`
	CheckedAt    *time.Time `json:"checkedAt"`
}

// Monitor asks the service for its health at an interval and keeps the
// answer, so that requests and jobs know whether to call it.
type Monitor struct {
	client   *Client
	model    string
	interval time.Duration

	mu          sync.Mutex
	status      Status
	faceState   State
	retryFloor  *float64
	onAvailable func()
}

// NewMonitor returns a monitor of the service, or of none if client is nil.
func NewMonitor(client *Client, model string) *Monitor {
	state := StateUnavailable
	if client == nil {
		state = StateDisabled
	}
	faceState := state
	if client != nil && client.token == "" {
		faceState = StateDisabled
	}
	return &Monitor{
		client:    client,
		model:     model,
		interval:  30 * time.Second,
		status:    Status{State: state, Model: model},
		faceState: faceState,
	}
}

// OnAvailable sets a function called when the service becomes available.
func (m *Monitor) OnAvailable(f func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onAvailable = f
}

// Status returns the last known state.
func (m *Monitor) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// Available reports whether the service answered with the configured model
// the last time it was asked.
func (m *Monitor) Available() bool {
	return m.Status().State == StateAvailable
}

// FacesAvailable reports whether the last health check advertised a supported
// face model. Face inference can run independently of the search model.
func (m *Monitor) FacesAvailable() bool {
	return m.FaceState() == StateAvailable
}

// RetryFloor is the least score of a face the contrast retry reported at the
// last successful health check, nil if unknown or the retry is off.
func (m *Monitor) RetryFloor() *float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.retryFloor
}

// FaceState is the last known state of face inference, independent of search.
func (m *Monitor) FaceState() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.faceState
}

// Run checks the service at once and then at the interval, until ctx ends.
func (m *Monitor) Run(ctx context.Context) {
	if m.client == nil {
		return
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		m.Check(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Check asks the service for its health now.
func (m *Monitor) Check(ctx context.Context) Status {
	if m.client == nil {
		return m.Status()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	health, err := m.client.Health(ctx)
	now := time.Now()
	next := Status{Model: m.model, CheckedAt: &now}
	switch {
	case err != nil:
		next.State = StateUnavailable
		next.Error = err.Error()
	case health.Model != m.model:
		next.State = StateMismatch
		next.ServiceModel = health.Model
		next.Device = health.Device
		next.Error = ErrModelMismatch.Error()
	default:
		next.State = StateAvailable
		next.ServiceModel = health.Model
		next.Device = health.Device
	}
	faceState := StateUnavailable
	switch {
	case m.client.token == "", err == nil && health.Faces == nil:
		faceState = StateDisabled
	case err == nil && health.Faces.supported():
		faceState = StateAvailable
	case err == nil:
		faceState = StateMismatch
	}
	if err == nil && health.Faces != nil {
		m.mu.Lock()
		m.retryFloor = health.Faces.RetryFloor
		m.mu.Unlock()
	}
	m.set(next, &faceState)
	return next
}

// Failed pauses search and embedding until the next successful health check.
func (m *Monitor) Failed(err error) {
	if m.client == nil {
		return
	}
	now := time.Now()
	m.set(Status{State: StateUnavailable, Model: m.model, Error: err.Error(), CheckedAt: &now}, nil)
}

func (m *Monitor) faceFailed() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.faceState = StateUnavailable
}

func (m *Monitor) set(next Status, faceState *State) {
	m.mu.Lock()
	previous := m.status.State
	m.status = next
	if faceState != nil {
		m.faceState = *faceState
	}
	onAvailable := m.onAvailable
	m.mu.Unlock()

	if previous == next.State {
		return
	}
	slog.Info("ML service state", "state", next.State, "model", next.Model, "service_model", next.ServiceModel, "error", next.Error)
	if next.State == StateAvailable && onAvailable != nil {
		onAvailable()
	}
}
