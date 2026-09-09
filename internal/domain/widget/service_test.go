package widget

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestServiceCreate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   Widget
		setup   func(*fakeRepository)
		wantErr error
	}{
		{
			name:  "creates a valid widget",
			input: Widget{Namespace: "default", Name: "w1", Size: "small"},
		},
		{
			name:    "rejects a widget without a size",
			input:   Widget{Namespace: "default", Name: "w1"},
			wantErr: nil, // validated below via errors.Is check on wrapped error
		},
		{
			name:  "propagates already-exists from the repository",
			input: Widget{Namespace: "default", Name: "w1", Size: "small"},
			setup: func(r *fakeRepository) {
				r.widgets[key("default", "w1")] = Widget{Namespace: "default", Name: "w1"}
			},
			wantErr: ErrAlreadyExists,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepository()
			if tt.setup != nil {
				tt.setup(repo)
			}
			svc := NewService(repo, fixedClock{now})

			got, err := svc.Create(context.Background(), tt.input)

			if tt.name == "rejects a widget without a size" {
				if err == nil {
					t.Fatal("expected a validation error, got nil")
				}
				return
			}

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.ResourceVersion != 1 {
				t.Errorf("got ResourceVersion %d, want 1", got.ResourceVersion)
			}
			if !got.CreatedAt.Equal(now) {
				t.Errorf("got CreatedAt %v, want %v", got.CreatedAt, now)
			}
			if got.Phase != PhasePending {
				t.Errorf("got Phase %q, want %q", got.Phase, PhasePending)
			}
		})
	}
}

func TestServiceUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		seed    Widget
		input   Widget
		wantErr error
	}{
		{
			name:  "updates when resource version matches",
			seed:  Widget{Namespace: "default", Name: "w1", Size: "small", ResourceVersion: 1},
			input: Widget{Namespace: "default", Name: "w1", Size: "large", ResourceVersion: 1},
		},
		{
			name:    "conflicts on stale resource version",
			seed:    Widget{Namespace: "default", Name: "w1", Size: "small", ResourceVersion: 2},
			input:   Widget{Namespace: "default", Name: "w1", Size: "large", ResourceVersion: 1},
			wantErr: ErrConflict,
		},
		{
			name:    "not found for missing widget",
			input:   Widget{Namespace: "default", Name: "missing", Size: "large"},
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := newFakeRepository()
			if tt.seed.Name != "" {
				repo.widgets[key(tt.seed.Namespace, tt.seed.Name)] = tt.seed
			}
			svc := NewService(repo, fixedClock{time.Now()})

			got, err := svc.Update(context.Background(), tt.input)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Size != tt.input.Size {
				t.Errorf("got Size %q, want %q", got.Size, tt.input.Size)
			}
		})
	}
}

func TestServiceDelete(t *testing.T) {
	t.Parallel()

	repo := newFakeRepository()
	repo.widgets[key("default", "w1")] = Widget{Namespace: "default", Name: "w1", ResourceVersion: 1}
	svc := NewService(repo, fixedClock{time.Now()})

	if _, err := svc.Delete(context.Background(), "default", "w1", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := svc.Get(context.Background(), "default", "w1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got error %v, want ErrNotFound", err)
	}
}
