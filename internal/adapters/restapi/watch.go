package restadapter

import (
	"context"

	"k8s.io/apimachinery/pkg/watch"

	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/domain/widget"
)

// eventWatcher adapts a domain event channel to apimachinery's
// watch.Interface, which is what the generic apiserver HTTP handlers use to
// stream watch responses to clients.
type eventWatcher struct {
	out    chan watch.Event
	cancel context.CancelFunc
}

func newEventWatcher(ctx context.Context, in <-chan widget.Event) *eventWatcher {
	ctx, cancel := context.WithCancel(ctx)
	w := &eventWatcher{
		out:    make(chan watch.Event),
		cancel: cancel,
	}
	go w.run(ctx, in)
	return w
}

func (w *eventWatcher) run(ctx context.Context, in <-chan widget.Event) {
	defer close(w.out)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-in:
			if !ok {
				return
			}
			var t watch.EventType
			switch ev.Type {
			case widget.EventAdded:
				t = watch.Added
			case widget.EventModified:
				t = watch.Modified
			case widget.EventDeleted:
				t = watch.Deleted
			default:
				continue
			}
			select {
			case w.out <- watch.Event{Type: t, Object: toExternal(&ev.Widget)}:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (w *eventWatcher) Stop()                          { w.cancel() }
func (w *eventWatcher) ResultChan() <-chan watch.Event { return w.out }
