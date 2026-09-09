package widget

import (
	"context"
	"fmt"
)

// Service implements the use cases (application layer) that orchestrate the
// domain model against the Repository port. This is what inbound adapters
// (the Kubernetes REST storage adapter) call - they never talk to the
// Repository directly, keeping the dependency graph pointing inward.
type Service struct {
	repo  Repository
	clock Clock
}

// NewService wires a Service against its Repository port. Accepting the
// interface (not a concrete Postgres type) is what makes this layer
// swappable and unit-testable with a fake.
func NewService(repo Repository, clock Clock) *Service {
	if clock == nil {
		clock = SystemClock
	}
	return &Service{repo: repo, clock: clock}
}

func (s *Service) Get(ctx context.Context, namespace, name string) (*Widget, error) {
	return s.repo.Get(ctx, namespace, name)
}

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Widget, error) {
	return s.repo.List(ctx, filter)
}

func (s *Service) Create(ctx context.Context, w Widget) (*Widget, error) {
	if err := w.Validate(); err != nil {
		return nil, fmt.Errorf("validate widget: %w", err)
	}
	now := s.clock.Now()
	w.CreatedAt = now
	w.UpdatedAt = now
	if w.Phase == "" {
		w.Phase = PhasePending
	}
	return s.repo.Create(ctx, &w)
}

func (s *Service) Update(ctx context.Context, w Widget) (*Widget, error) {
	if err := w.Validate(); err != nil {
		return nil, fmt.Errorf("validate widget: %w", err)
	}
	w.UpdatedAt = s.clock.Now()
	return s.repo.Update(ctx, &w)
}

func (s *Service) Delete(ctx context.Context, namespace, name string, expectedResourceVersion uint64) (*Widget, error) {
	return s.repo.Delete(ctx, namespace, name, expectedResourceVersion)
}

func (s *Service) Watch(ctx context.Context, namespace string, resourceVersion uint64) (<-chan Event, error) {
	return s.repo.Watch(ctx, namespace, resourceVersion)
}
