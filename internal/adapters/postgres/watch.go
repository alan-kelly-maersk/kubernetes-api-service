package postgres

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/domain/widget"
)

// Watch opens a dedicated connection and LISTENs on notifyChannel,
// forwarding matching events to the returned channel until ctx is done.
//
// resourceVersion is accepted for API compatibility with watch.Interface
// semantics but this simple template does not replay history - it only
// streams events from "now". Production use should either replay from a
// durable outbox table or document this limitation to clients.
func (r *Repository) Watch(ctx context.Context, namespace string, _ uint64) (<-chan widget.Event, error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(ctx, "LISTEN "+notifyChannel); err != nil {
		conn.Release()
		return nil, err
	}

	out := make(chan widget.Event)
	go func() {
		defer conn.Release()
		defer close(out)
		for {
			notification, err := conn.Conn().WaitForNotification(ctx)
			if err != nil {
				return // ctx cancelled or connection lost
			}
			var payload watchPayload
			if err := json.Unmarshal([]byte(notification.Payload), &payload); err != nil {
				slog.Warn("discarding malformed widget notification", "error", err)
				continue
			}
			if namespace != "" && payload.Widget.Namespace != namespace {
				continue
			}
			select {
			case out <- widget.Event{Type: payload.Type, Widget: payload.Widget}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
