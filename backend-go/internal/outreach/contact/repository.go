package contact

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository interface {
	// UpsertFromBusiness finds or creates the contact for (userID, businessID),
	// pulling name/email/phone defaults from the business on create.
	UpsertFromBusiness(ctx context.Context, userID, businessID uuid.UUID) (*domain.Contact, error)

	Get(ctx context.Context, userID, id uuid.UUID) (*domain.Contact, error)
	// GetByID looks up a contact by primary key only — used by the public
	// unsubscribe handler where the user_id is not known up front.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Contact, error)
	GetByBusiness(ctx context.Context, userID, businessID uuid.UUID) (*domain.Contact, error)
	List(ctx context.Context, userID uuid.UUID, stage string, limit, offset int) ([]domain.Contact, int, error)
	Update(ctx context.Context, c domain.Contact) (*domain.Contact, error)
	// MarkUnsubscribed flips the suppression flag. Idempotent; safe on a
	// contact that was already unsubscribed.
	MarkUnsubscribed(ctx context.Context, contactID uuid.UUID, reason string) error
	// BulkUpdate applies the same field changes to many contacts in one tx.
	// Used by the lead-list bulk-action UI in P3.
	BulkUpdate(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, fields BulkFields) (int, error)
}

// BulkFields describes the fields settable in a bulk update. Nil fields mean
// "leave unchanged"; non-nil means "set to this value".
type BulkFields struct {
	PipelineStage     *string
	DefaultAutomation *string
	DefaultSequenceID *uuid.UUID
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) UpsertFromBusiness(ctx context.Context, userID, businessID uuid.UUID) (*domain.Contact, error) {
	// Fast path — already exists.
	if existing, err := r.GetByBusiness(ctx, userID, businessID); err == nil {
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	// Load business defaults.
	var (
		name  string
		email *string
		phone *string
	)
	err := r.pool.QueryRow(ctx,
		`SELECT name, email, phone FROM businesses WHERE id = $1`, businessID,
	).Scan(&name, &email, &phone)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load business for contact: %w", err)
	}

	// Insert — ON CONFLICT handles race with another concurrent upsert.
	c := domain.Contact{
		ID:                uuid.New(),
		UserID:            userID,
		BusinessID:        businessID,
		PrimaryEmail:      email,
		PrimaryPhone:      phone,
		DisplayName:       name,
		PipelineStage:     domain.PipelineLead,
		DefaultAutomation: domain.AutomationManual,
	}
	_, err = r.pool.Exec(ctx,
		`INSERT INTO contacts (id, user_id, business_id, primary_email, primary_phone,
		                       display_name, pipeline_stage, default_automation)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (user_id, business_id) DO NOTHING`,
		c.ID, c.UserID, c.BusinessID, c.PrimaryEmail, c.PrimaryPhone,
		c.DisplayName, c.PipelineStage, c.DefaultAutomation,
	)
	if err != nil {
		return nil, fmt.Errorf("insert contact: %w", err)
	}
	return r.GetByBusiness(ctx, userID, businessID)
}

func (r *repository) Get(ctx context.Context, userID, id uuid.UUID) (*domain.Contact, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, business_id, primary_email, primary_phone, display_name,
		        pipeline_stage, default_automation, default_sequence_id,
		        unsubscribed_at, unsubscribe_reason, created_at, updated_at
		 FROM contacts WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
}

func (r *repository) GetByBusiness(ctx context.Context, userID, businessID uuid.UUID) (*domain.Contact, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, business_id, primary_email, primary_phone, display_name,
		        pipeline_stage, default_automation, default_sequence_id,
		        unsubscribed_at, unsubscribe_reason, created_at, updated_at
		 FROM contacts WHERE user_id = $1 AND business_id = $2`,
		userID, businessID,
	)
}

func (r *repository) scanOne(ctx context.Context, q string, args ...any) (*domain.Contact, error) {
	var c domain.Contact
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&c.ID, &c.UserID, &c.BusinessID, &c.PrimaryEmail, &c.PrimaryPhone, &c.DisplayName,
		&c.PipelineStage, &c.DefaultAutomation, &c.DefaultSequenceID,
		&c.UnsubscribedAt, &c.UnsubscribeReason, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan contact: %w", err)
	}
	return &c, nil
}

func (r *repository) List(ctx context.Context, userID uuid.UUID, stage string, limit, offset int) ([]domain.Contact, int, error) {
	stageFilter := ""
	args := []any{userID}
	if stage != "" {
		stageFilter = "AND pipeline_stage = $2"
		args = append(args, stage)
	}
	var total int
	if err := r.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM contacts WHERE user_id = $1 %s", stageFilter),
		args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count contacts: %w", err)
	}

	// Append limit+offset args.
	limitIdx := len(args) + 1
	offsetIdx := limitIdx + 1
	args = append(args, limit, offset)

	q := fmt.Sprintf(
		`SELECT id, user_id, business_id, primary_email, primary_phone, display_name,
		        pipeline_stage, default_automation, default_sequence_id,
		        unsubscribed_at, unsubscribe_reason, created_at, updated_at
		 FROM contacts WHERE user_id = $1 %s
		 ORDER BY updated_at DESC
		 LIMIT $%d OFFSET $%d`, stageFilter, limitIdx, offsetIdx,
	)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close()

	var out []domain.Contact
	for rows.Next() {
		var c domain.Contact
		if err := rows.Scan(&c.ID, &c.UserID, &c.BusinessID, &c.PrimaryEmail, &c.PrimaryPhone,
			&c.DisplayName, &c.PipelineStage, &c.DefaultAutomation, &c.DefaultSequenceID,
			&c.UnsubscribedAt, &c.UnsubscribeReason, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan contact row: %w", err)
		}
		out = append(out, c)
	}
	return out, total, nil
}

func (r *repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Contact, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, business_id, primary_email, primary_phone, display_name,
		        pipeline_stage, default_automation, default_sequence_id,
		        unsubscribed_at, unsubscribe_reason, created_at, updated_at
		 FROM contacts WHERE id = $1`,
		id,
	)
}

func (r *repository) MarkUnsubscribed(ctx context.Context, contactID uuid.UUID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE contacts SET unsubscribed_at = COALESCE(unsubscribed_at, now()),
		                     unsubscribe_reason = COALESCE(unsubscribe_reason, $2),
		                     updated_at = now()
		 WHERE id = $1`,
		contactID, reason,
	)
	return err
}

func (r *repository) BulkUpdate(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, fields BulkFields) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// Build a dynamic SET clause covering only the non-nil fields.
	sets := []string{"updated_at = now()"}
	args := []any{userID, ids}
	idx := 3
	if fields.PipelineStage != nil {
		sets = append(sets, fmt.Sprintf("pipeline_stage = $%d", idx))
		args = append(args, *fields.PipelineStage)
		idx++
	}
	if fields.DefaultAutomation != nil {
		sets = append(sets, fmt.Sprintf("default_automation = $%d", idx))
		args = append(args, *fields.DefaultAutomation)
		idx++
	}
	if fields.DefaultSequenceID != nil {
		sets = append(sets, fmt.Sprintf("default_sequence_id = $%d", idx))
		args = append(args, *fields.DefaultSequenceID)
		idx++
	}
	if len(sets) == 1 {
		// Only updated_at would change — no-op.
		return 0, nil
	}
	q := fmt.Sprintf(
		`UPDATE contacts SET %s WHERE user_id = $1 AND id = ANY($2)`,
		joinComma(sets),
	)
	tag, err := r.pool.Exec(ctx, q, args...)
	if err != nil {
		return 0, fmt.Errorf("bulk update contacts: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// joinComma joins SQL snippets with ", ". Tiny helper, kept here so we don't
// pull in a one-liner from the standard library and pollute imports.
func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

func (r *repository) Update(ctx context.Context, c domain.Contact) (*domain.Contact, error) {
	_, err := r.pool.Exec(ctx,
		`UPDATE contacts SET
		   primary_email = $1,
		   primary_phone = $2,
		   display_name = $3,
		   pipeline_stage = $4,
		   default_automation = $5,
		   default_sequence_id = $6,
		   updated_at = now()
		 WHERE id = $7 AND user_id = $8`,
		c.PrimaryEmail, c.PrimaryPhone, c.DisplayName, c.PipelineStage,
		c.DefaultAutomation, c.DefaultSequenceID, c.ID, c.UserID,
	)
	if err != nil {
		return nil, fmt.Errorf("update contact: %w", err)
	}
	return r.Get(ctx, c.UserID, c.ID)
}
