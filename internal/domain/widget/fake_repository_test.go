package widget
package widget

import (
	"context"
	"time"
)

// fakeRepository is a hand-written in-memory stand-in for Repository,
// used only in tests. No mocking framework required: it is small enough
// to write and read directly, and it lets tests assert real behavior
// (e.g. optimistic concurrency) rather than just "was called with X".
type fakeRepository struct {
	widgets map[string]Widget // key: namespace/name
	getErr  error
	nextErr error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{widgets: make(map[string]Widget)}
}

func key(namespace, name string) string { return namespace + "/" + name }

func (f *fakeRepository) Get(_ context.Context, namespace, name string) (*Widget, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	w, ok := f.widgets[key(namespace, name)]
	if !ok {
		return nil, ErrNotFound
	}
	cp := w
	return &cp, nil
}

func (f *fakeRepository) List(_ context.Context, filter ListFilter) ([]Widget, error) {
	var out []Widget
	for _, w := range f.widgets {
		if filter.Namespace != "" && w.Namespace != filter.Namespace {
			continue
		}
		out = append(out, w)
	}
	return out, nil
}

func (f *fakeRepository) Create(_ context.Context, w *Widget) (*Widget, error) {
	if f.nextErr != nil {
		err := f.nextErr
		f.nextErr = nil
		return nil, err
	}
	k := key(w.Namespace, w.Name)
	if _, exists := f.widgets[k]; exists {
		return nil, ErrAlreadyExists
	}
	cp := *w
	cp.ResourceVersion = 1
	f.widgets[k] = cp
	out := cp
	return &out, nil
}

func (f *fakeRepository) Update(_ context.Context, w *Widget) (*Widget, error) {
	k := key(w.Namespace, w.Name)
	existing, ok := f.widgets[k]
	if !ok {
		return nil, ErrNotFound
	}
	if w.ResourceVersion != 0 && w.ResourceVersion != existing.ResourceVersion {
		return nil, ErrConflict
	}
	cp := *w
	cp.ResourceVersion = existing.ResourceVersion + 1
	f.widgets[k] = cp
	out := cp
	return &out, nil
}

func (f *fakeRepository) Delete(_ context.Context, namespace, name string, expectedResourceVersion uint64) (*Widget, error) {
	k := key(namespace, name)
	existing, ok := f.widgets[k]
	if !ok {
		return nil, ErrNotFound
	}
	if expectedResourceVersion != 0 && expectedResourceVersion != existing.ResourceVersion {
		return nil, ErrConflict
	}
	delete(f.widgets, k)
	return &existing, nil
}

func (f *fakeRepository) Watch(_ context.Context, _ string, _ uint64) (<-chan Event, error) {
	ch := make(chan Event)
	close(ch)
	return ch, nil
}

// fixedClock lets tests assert exact timestamps instead of "close to now".
type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }
