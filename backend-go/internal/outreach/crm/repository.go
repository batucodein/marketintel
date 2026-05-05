// Package crm hosts the smaller CRM endpoints that don't have their own
// home: tasks (with optional contact/conversation link, due date, completed
// flag) and free-text notes pinned to a contact.
package crm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository interface {
	// Tasks
	CreateTask(ctx context.Context, t domain.Task) (*domain.Task, error)
	GetTask(ctx context.Context, userID, id uuid.UUID) (*domain.Task, error)
	ListTasks(ctx context.Context, userID uuid.UUID, contactID *uuid.UUID, openOnly bool) ([]domain.Task, error)
	UpdateTask(ctx context.Context, t domain.Task) (*domain.Task, error)
	DeleteTask(ctx context.Context, userID, id uuid.UUID) error
	OverdueCount(ctx context.Context, userID uuid.UUID) (int, error)

	// Notes
	CreateNote(ctx context.Context, n domain.Note) (*domain.Note, error)
	ListNotes(ctx context.Context, userID, contactID uuid.UUID) ([]domain.Note, error)
	DeleteNote(ctx context.Context, userID, id uuid.UUID) error
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository { return &repository{pool: pool} }

func (r *repository) CreateTask(ctx context.Context, t domain.Task) (*domain.Task, error) {
	t.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tasks (id, user_id, contact_id, conversation_id, title, body, due_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.ID, t.UserID, t.ContactID, t.ConversationID, t.Title, t.Body, t.DueAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return r.GetTask(ctx, t.UserID, t.ID)
}

func (r *repository) GetTask(ctx context.Context, userID, id uuid.UUID) (*domain.Task, error) {
	var t domain.Task
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, contact_id, conversation_id, title, body, due_at, completed_at, created_at, updated_at
		 FROM tasks WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&t.ID, &t.UserID, &t.ContactID, &t.ConversationID, &t.Title, &t.Body, &t.DueAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *repository) ListTasks(ctx context.Context, userID uuid.UUID, contactID *uuid.UUID, openOnly bool) ([]domain.Task, error) {
	args := []any{userID}
	q := `SELECT id, user_id, contact_id, conversation_id, title, body, due_at, completed_at, created_at, updated_at
	      FROM tasks WHERE user_id = $1`
	idx := 2
	if contactID != nil {
		q += fmt.Sprintf(" AND contact_id = $%d", idx)
		args = append(args, *contactID)
		idx++
	}
	if openOnly {
		q += " AND completed_at IS NULL"
	}
	q += " ORDER BY completed_at NULLS FIRST, due_at NULLS LAST, created_at DESC"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Task
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.UserID, &t.ContactID, &t.ConversationID, &t.Title, &t.Body, &t.DueAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *repository) UpdateTask(ctx context.Context, t domain.Task) (*domain.Task, error) {
	_, err := r.pool.Exec(ctx,
		`UPDATE tasks SET
		   title = $1, body = $2, due_at = $3, completed_at = $4, updated_at = now()
		 WHERE id = $5 AND user_id = $6`,
		t.Title, t.Body, t.DueAt, t.CompletedAt, t.ID, t.UserID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetTask(ctx, t.UserID, t.ID)
}

func (r *repository) DeleteTask(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (r *repository) OverdueCount(ctx context.Context, userID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM tasks WHERE user_id = $1 AND completed_at IS NULL AND due_at < $2`,
		userID, time.Now(),
	).Scan(&n)
	return n, err
}

func (r *repository) CreateNote(ctx context.Context, n domain.Note) (*domain.Note, error) {
	n.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notes (id, user_id, contact_id, body) VALUES ($1, $2, $3, $4)`,
		n.ID, n.UserID, n.ContactID, n.Body,
	)
	if err != nil {
		return nil, err
	}
	var out domain.Note
	err = r.pool.QueryRow(ctx,
		`SELECT id, user_id, contact_id, body, created_at FROM notes WHERE id = $1`, n.ID,
	).Scan(&out.ID, &out.UserID, &out.ContactID, &out.Body, &out.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *repository) ListNotes(ctx context.Context, userID, contactID uuid.UUID) ([]domain.Note, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, contact_id, body, created_at FROM notes
		 WHERE user_id = $1 AND contact_id = $2 ORDER BY created_at DESC`,
		userID, contactID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Note
	for rows.Next() {
		var n domain.Note
		if err := rows.Scan(&n.ID, &n.UserID, &n.ContactID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func (r *repository) DeleteNote(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM notes WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}
