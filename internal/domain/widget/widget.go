// Package widget contains the core domain model and business rules for the
// Widget resource. It has zero dependencies on Kubernetes machinery or any
// specific datastore - those concerns live in the adapters layer.
package widget

import (
	"context"
	"errors"
	"time"
)

// Widget is the domain entity - the thing the business actually cares about.
type Widget struct {
	// Identity
	Namespace string
	Name      string
	UID       string

	// Optimistic concurrency token, opaque to the domain.
	ResourceVersion uint64

	// Spec/Status are intentionally simple; extend per real requirements.
	Size  string
	Color string

	Phase   Phase
	Message string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// Phase models the lifecycle of a Widget.
type Phase string

const (
	PhasePending Phase = "Pending"
	PhaseReady   Phase = "Ready"
	PhaseFailed  Phase = "Failed"
)

var (
	// ErrNotFound is returned by a Repository when no matching Widget exists.
	ErrNotFound = errors.New("widget not found")
	// ErrConflict is returned when an update is attempted against a stale
	// ResourceVersion (optimistic concurrency failure).
	ErrConflict = errors.New("widget resource version conflict")
	// ErrAlreadyExists is returned when creating a Widget that already exists.
	ErrAlreadyExists = errors.New("widget already exists")
)

// Validate applies domain invariants. Kept independent of Kubernetes
// validation so the rules are reusable and unit-testable in isolation.
func (w *Widget) Validate() error {
	if w.Name == "" {
		return errors.New("name is required")
	}
	if w.Size == "" {
		return errors.New("size is required")
	}
	return nil
}

// ListFilter describes the query parameters accepted when listing widgets.
type ListFilter struct {
	Namespace string
	Limit     int
	Continue  string
}

// Event describes a domain change published to watchers.
type EventType string

const (
	EventAdded    EventType = "ADDED"
	EventModified EventType = "MODIFIED"
	EventDeleted  EventType = "DELETED"
)

type Event struct {
	Type   EventType
	Widget Widget
}

// Repository is the outbound port: how the domain persists and retrieves
// widgets. Adapters (e.g. Postgres) implement this interface; the domain
// and use cases only ever depend on this abstraction (Dependency Inversion).
type Repository interface {
	Get(ctx context.Context, namespace, name string) (*Widget, error)
	List(ctx context.Context, filter ListFilter) ([]Widget, error)
	Create(ctx context.Context, w *Widget) (*Widget, error)
	Update(ctx context.Context, w *Widget) (*Widget, error)
	Delete(ctx context.Context, namespace, name string, expectedResourceVersion uint64) (*Widget, error)

	// Watch streams change events starting after resourceVersion (0 means
	// "now"). The returned channel is closed when ctx is cancelled.
	Watch(ctx context.Context, namespace string, resourceVersion uint64) (<-chan Event, error)
}

// Clock is a small port allowing time to be controlled in tests.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// SystemClock is the production Clock implementation.
var SystemClock Clock = systemClock{}
