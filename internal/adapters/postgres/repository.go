// Package postgres is the driven adapter implementing widget.Repository
// against PostgreSQL. It is the only package in this service allowed to
// know about SQL - swap it out (e.g. for a different store) without
// touching the domain or the Kubernetes REST adapter.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alan-kelly-maersk/kubernetes-api-service/internal/domain/widget"
)

const notifyChannel = "widget_events"

// Repository implements widget.Repository backed by a Postgres table.
type Repository struct {
	pool *pgxpool.Pool
}

// New builds a Repository from a ready connection pool.
func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

var _ widget.Repository = (*Repository)(nil)

func (r *Repository) Get(ctx context.Context, namespace, name string) (*widget.Widget, error) {
	const q = `
		SELECT namespace, name, uid, resource_version, size, color, phase, message,
		       created_at, updated_at, deleted_at
		FROM widgets
		WHERE namespace = $1 AND name = $2 AND deleted_at IS NULL`
	row := r.pool.QueryRow(ctx, q, namespace, name)
	w, err := scanWidget(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, widget.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get widget: %w", err)
	}
	return w, nil
}

func (r *Repository) List(ctx context.Context, filter widget.ListFilter) ([]widget.Widget, error) {
	q := `
		SELECT namespace, name, uid, resource_version, size, color, phase, message,
		       created_at, updated_at, deleted_at
		FROM widgets
		WHERE deleted_at IS NULL`
	args := []any{}
	if filter.Namespace != "" {
		args = append(args, filter.Namespace)
		q += fmt.Sprintf(" AND namespace = $%d", len(args))
	}
	q += " ORDER BY namespace, name"
	if filter.Limit > 0 {
		args = append(args, filter.Limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list widgets: %w", err)
	}
	defer rows.Close()

	var out []widget.Widget
	for rows.Next() {
		w, err := scanWidget(rows)
		if err != nil {
			return nil, fmt.Errorf("scan widget: %w", err)
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (r *Repository) Create(ctx context.Context, w *widget.Widget) (*widget.Widget, error) {
	const q = `
		INSERT INTO widgets (namespace, name, uid, resource_version, size, color, phase, message, created_at, updated_at)
		VALUES ($1, $2, gen_random_uuid(), 1, $3, $4, $5, $6, $7, $7)
		RETURNING namespace, name, uid, resource_version, size, color, phase, message, created_at, updated_at, deleted_at`
	row := r.pool.QueryRow(ctx, q, w.Namespace, w.Name, w.Size, w.Color, string(w.Phase), w.Message, w.CreatedAt)
	created, err := scanWidget(row)
	if isUniqueViolation(err) {
		return nil, widget.ErrAlreadyExists
	}
	if err != nil {
		return nil, fmt.Errorf("create widget: %w", err)
	}
	r.notify(ctx, widget.EventAdded, created)
	return created, nil
}

func (r *Repository) Update(ctx context.Context, w *widget.Widget) (*widget.Widget, error) {
	const q = `
		UPDATE widgets
		SET size = $3, color = $4, phase = $5, message = $6, updated_at = $7, resource_version = resource_version + 1
		WHERE namespace = $1 AND name = $2 AND resource_version = $8 AND deleted_at IS NULL
		RETURNING namespace, name, uid, resource_version, size, color, phase, message, created_at, updated_at, deleted_at`
	row := r.pool.QueryRow(ctx, q, w.Namespace, w.Name, w.Size, w.Color, string(w.Phase), w.Message, w.UpdatedAt, w.ResourceVersion)
	updated, err := scanWidget(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Distinguish "doesn't exist" from "version mismatch" for a clearer error.
		if _, getErr := r.Get(ctx, w.Namespace, w.Name); errors.Is(getErr, widget.ErrNotFound) {
			return nil, widget.ErrNotFound
		}
		return nil, widget.ErrConflict
	}
	if err != nil {
		return nil, fmt.Errorf("update widget: %w", err)
	}
	r.notify(ctx, widget.EventModified, updated)
	return updated, nil
}

func (r *Repository) Delete(ctx context.Context, namespace, name string, expectedResourceVersion uint64) (*widget.Widget, error) {
	current, err := r.Get(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if expectedResourceVersion != 0 && current.ResourceVersion != expectedResourceVersion {
		return nil, widget.ErrConflict
	}
	const q = `UPDATE widgets SET deleted_at = now() WHERE namespace = $1 AND name = $2 AND deleted_at IS NULL`
	if _, err := r.pool.Exec(ctx, q, namespace, name); err != nil {
		return nil, fmt.Errorf("delete widget: %w", err)
	}
	r.notify(ctx, widget.EventDeleted, current)
	return current, nil
}

// row is satisfied by both pgx.Row and pgx.Rows.
type row interface {
	Scan(dest ...any) error
}

func scanWidget(rw row) (*widget.Widget, error) {
	var (
		w         widget.Widget
		phase     string
		deletedAt *time.Time
	)
	if err := rw.Scan(&w.Namespace, &w.Name, &w.UID, &w.ResourceVersion, &w.Size, &w.Color,
		&phase, &w.Message, &w.CreatedAt, &w.UpdatedAt, &deletedAt); err != nil {
		return nil, err
	}
	w.Phase = widget.Phase(phase)
	w.DeletedAt = deletedAt
	return &w, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && err.Error() != "" && (containsCode(err, "23505"))
}

func containsCode(err error, code string) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == code
	}
	return false
}

// notify publishes a change via Postgres LISTEN/NOTIFY so Watch() calls
// (including from other replicas) can observe it. Payload is small and
// self-contained on purpose; watchers re-fetch nothing else.
func (r *Repository) notify(ctx context.Context, evType widget.EventType, w *widget.Widget) {
	payload, err := json.Marshal(watchPayload{Type: evType, Widget: *w})
	if err != nil {
		return
	}
	// Best-effort: a failed notify should not fail the write that triggered it.
	_, _ = r.pool.Exec(ctx, "SELECT pg_notify($1, $2)", notifyChannel, string(payload))
}

type watchPayload struct {
	Type   widget.EventType `json:"type"`
	Widget widget.Widget    `json:"widget"`
}
