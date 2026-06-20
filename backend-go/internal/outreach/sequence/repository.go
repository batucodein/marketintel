// Package sequence implements follow-up playbooks. A sequence is a list of
// timed steps; a sequence_run tracks one conversation's progress through
// that playbook. The engine (engine.go) is invoked from the scheduler tick.
package sequence

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
	// Sequences
	CreateSequence(ctx context.Context, s domain.Sequence) (*domain.Sequence, error)
	GetSequence(ctx context.Context, userID, id uuid.UUID) (*domain.Sequence, error)
	ListSequences(ctx context.Context, userID uuid.UUID) ([]domain.Sequence, error)
	UpdateSequence(ctx context.Context, s domain.Sequence) (*domain.Sequence, error)
	DeleteSequence(ctx context.Context, userID, id uuid.UUID) error

	// Steps
	ReplaceSteps(ctx context.Context, sequenceID uuid.UUID, steps []domain.SequenceStep) error
	ListSteps(ctx context.Context, sequenceID uuid.UUID) ([]domain.SequenceStep, error)
	GetStep(ctx context.Context, sequenceID uuid.UUID, stepNumber int) (*domain.SequenceStep, error)

	// Runs
	StartRun(ctx context.Context, sequenceID, conversationID uuid.UUID, firstAt time.Time) (*domain.SequenceRun, error)
	GetRun(ctx context.Context, id uuid.UUID) (*domain.SequenceRun, error)
	DueRuns(ctx context.Context, now time.Time, limit int) ([]domain.SequenceRun, error)
	AdvanceRun(ctx context.Context, id uuid.UUID, nextStep int, nextRunAt time.Time) error
	BumpRun(ctx context.Context, id uuid.UUID, nextRunAt time.Time) error
	CompleteRun(ctx context.Context, id uuid.UUID, status string) error
	StopRunsByConversation(ctx context.Context, conversationID uuid.UUID) error
	ActiveRunsCount(ctx context.Context, sequenceID uuid.UUID) (int, error)
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository { return &repository{pool: pool} }

func (r *repository) CreateSequence(ctx context.Context, s domain.Sequence) (*domain.Sequence, error) {
	s.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sequences (id, user_id, name, description, is_template)
		 VALUES ($1, $2, $3, $4, $5)`,
		s.ID, s.UserID, s.Name, s.Description, s.IsTemplate,
	)
	if err != nil {
		return nil, fmt.Errorf("insert sequence: %w", err)
	}
	return r.GetSequence(ctx, s.UserID, s.ID)
}

func (r *repository) GetSequence(ctx context.Context, userID, id uuid.UUID) (*domain.Sequence, error) {
	var s domain.Sequence
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, description, is_template, created_at, updated_at
		 FROM sequences WHERE id = $1 AND user_id = $2`, id, userID,
	).Scan(&s.ID, &s.UserID, &s.Name, &s.Description, &s.IsTemplate, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan sequence: %w", err)
	}
	return &s, nil
}

func (r *repository) ListSequences(ctx context.Context, userID uuid.UUID) ([]domain.Sequence, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, name, description, is_template, created_at, updated_at
		 FROM sequences WHERE user_id = $1 OR is_template = true ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Sequence
	for rows.Next() {
		var s domain.Sequence
		if err := rows.Scan(&s.ID, &s.UserID, &s.Name, &s.Description, &s.IsTemplate, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *repository) UpdateSequence(ctx context.Context, s domain.Sequence) (*domain.Sequence, error) {
	_, err := r.pool.Exec(ctx,
		`UPDATE sequences SET name = $1, description = $2, updated_at = now()
		 WHERE id = $3 AND user_id = $4`,
		s.Name, s.Description, s.ID, s.UserID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetSequence(ctx, s.UserID, s.ID)
}

func (r *repository) DeleteSequence(ctx context.Context, userID, id uuid.UUID) error {
	// Block deletion when active runs exist.
	var n int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM sequence_runs WHERE sequence_id = $1 AND status = 'active'`, id,
	).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("cannot delete: %d active run(s)", n)
	}
	_, err := r.pool.Exec(ctx, `DELETE FROM sequences WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

func (r *repository) ReplaceSteps(ctx context.Context, sequenceID uuid.UUID, steps []domain.SequenceStep) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM sequence_steps WHERE sequence_id = $1`, sequenceID); err != nil {
		return err
	}
	for _, st := range steps {
		// A cadence step is structurally the "no reply" path — replies are
		// handled by the campaign reply branch, never by a step trigger. Reject
		// anything else loudly instead of silently rewriting the user's config.
		if st.Trigger != "" && st.Trigger != domain.SequenceTriggerNoReply {
			return fmt.Errorf("step %d: trigger %q is not supported — cadence steps fire on no_reply only (replies are handled by the campaign reply branch)", st.StepNumber, st.Trigger)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO sequence_steps (id, sequence_id, step_number, wait_days, trigger,
			                              action, prompt_override, auto_send)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7)`,
			sequenceID, st.StepNumber, st.WaitDays, domain.SequenceTriggerNoReply, st.Action,
			st.PromptOverride, st.AutoSend,
		); err != nil {
			return fmt.Errorf("insert step %d: %w", st.StepNumber, err)
		}
	}
	return tx.Commit(ctx)
}

func (r *repository) ListSteps(ctx context.Context, sequenceID uuid.UUID) ([]domain.SequenceStep, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, sequence_id, step_number, wait_days, trigger, action, prompt_override, auto_send
		 FROM sequence_steps WHERE sequence_id = $1 ORDER BY step_number`, sequenceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SequenceStep
	for rows.Next() {
		var s domain.SequenceStep
		if err := rows.Scan(&s.ID, &s.SequenceID, &s.StepNumber, &s.WaitDays, &s.Trigger, &s.Action, &s.PromptOverride, &s.AutoSend); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (r *repository) GetStep(ctx context.Context, sequenceID uuid.UUID, stepNumber int) (*domain.SequenceStep, error) {
	var s domain.SequenceStep
	err := r.pool.QueryRow(ctx,
		`SELECT id, sequence_id, step_number, wait_days, trigger, action, prompt_override, auto_send
		 FROM sequence_steps WHERE sequence_id = $1 AND step_number = $2`,
		sequenceID, stepNumber,
	).Scan(&s.ID, &s.SequenceID, &s.StepNumber, &s.WaitDays, &s.Trigger, &s.Action, &s.PromptOverride, &s.AutoSend)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *repository) StartRun(ctx context.Context, sequenceID, conversationID uuid.UUID, firstAt time.Time) (*domain.SequenceRun, error) {
	id := uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sequence_runs (id, sequence_id, conversation_id, current_step, next_run_at, status)
		 VALUES ($1, $2, $3, 1, $4, 'active')
		 ON CONFLICT (sequence_id, conversation_id) DO NOTHING`,
		id, sequenceID, conversationID, firstAt,
	)
	if err != nil {
		return nil, err
	}
	return r.GetRun(ctx, id)
}

func (r *repository) GetRun(ctx context.Context, id uuid.UUID) (*domain.SequenceRun, error) {
	var run domain.SequenceRun
	err := r.pool.QueryRow(ctx,
		`SELECT id, sequence_id, conversation_id, current_step, next_run_at, status, last_error, created_at, updated_at
		 FROM sequence_runs WHERE id = $1`, id,
	).Scan(&run.ID, &run.SequenceID, &run.ConversationID, &run.CurrentStep, &run.NextRunAt, &run.Status, &run.LastError, &run.CreatedAt, &run.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *repository) DueRuns(ctx context.Context, now time.Time, limit int) ([]domain.SequenceRun, error) {
	// Atomically CLAIM the due runs by pushing next_run_at forward 90s as we
	// select them. Overlapping scheduler ticks (Cloud Scheduler retries, slow
	// ticks) would otherwise pick up the same runs and double-fire actions
	// (duplicate drafts, duplicate auto-sent emails). processOne overwrites
	// next_run_at via Bump/Advance/Complete, so the claim never sticks.
	rows, err := r.pool.Query(ctx,
		`UPDATE sequence_runs SET next_run_at = $1 + interval '90 seconds', updated_at = now()
		 WHERE id IN (
		   SELECT sr.id FROM sequence_runs sr
		    WHERE sr.status = 'active' AND sr.next_run_at <= $1
		      -- Don't fire follow-ups for a paused/ended group. Runs whose
		      -- conversation has no campaign, or an active campaign, still fire.
		      -- While paused the run isn't claimed, so its next_run_at stays
		      -- frozen — Resume shifts it forward by the pause duration.
		      AND NOT EXISTS (
		        SELECT 1 FROM conversations cv JOIN campaigns ca ON ca.id = cv.campaign_id
		         WHERE cv.id = sr.conversation_id AND ca.status <> 'active'
		      )
		    ORDER BY sr.next_run_at LIMIT $2
		    FOR UPDATE SKIP LOCKED
		 )
		 RETURNING id, sequence_id, conversation_id, current_step, next_run_at, status, last_error, created_at, updated_at`,
		now, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SequenceRun
	for rows.Next() {
		var run domain.SequenceRun
		if err := rows.Scan(&run.ID, &run.SequenceID, &run.ConversationID, &run.CurrentStep, &run.NextRunAt, &run.Status, &run.LastError, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}

func (r *repository) AdvanceRun(ctx context.Context, id uuid.UUID, nextStep int, nextRunAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sequence_runs SET current_step = $1, next_run_at = $2, updated_at = now()
		 WHERE id = $3`,
		nextStep, nextRunAt, id,
	)
	return err
}

func (r *repository) BumpRun(ctx context.Context, id uuid.UUID, nextRunAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sequence_runs SET next_run_at = $1, updated_at = now() WHERE id = $2`,
		nextRunAt, id,
	)
	return err
}

func (r *repository) CompleteRun(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sequence_runs SET status = $1, updated_at = now() WHERE id = $2`,
		status, id,
	)
	return err
}

func (r *repository) StopRunsByConversation(ctx context.Context, conversationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sequence_runs SET status = 'stopped_on_reply', updated_at = now()
		 WHERE conversation_id = $1 AND status = 'active'`,
		conversationID,
	)
	return err
}

func (r *repository) ActiveRunsCount(ctx context.Context, sequenceID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM sequence_runs WHERE sequence_id = $1 AND status = 'active'`, sequenceID,
	).Scan(&n)
	return n, err
}
