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

	// EnsureBulkFromBusinesses processes a list of business IDs and bucketing
	// them into added/already_existed/no_email — used by the leads page's
	// "Add to contacts" bulk action. Idempotent: re-running picks up newly
	// emailed leads as "added" and existing contacts as "already_existed".
	EnsureBulkFromBusinesses(ctx context.Context, userID uuid.UUID, businessIDs []uuid.UUID) (BulkEnsureResult, error)

	Get(ctx context.Context, userID, id uuid.UUID) (*domain.Contact, error)
	// GetByID looks up a contact by primary key only — used by the public
	// unsubscribe handler where the user_id is not known up front.
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Contact, error)
	GetByBusiness(ctx context.Context, userID, businessID uuid.UUID) (*domain.Contact, error)
	List(ctx context.Context, userID uuid.UUID, stage string, limit, offset int) ([]domain.Contact, int, error)
	// ListByMarket returns the user's contacts whose business belongs to the
	// given market (via the business_markets junction). Contacts stay
	// one-per-(user,business) globally — market membership is derived, not
	// stored on the contact, so dedup is preserved.
	ListByMarket(ctx context.Context, userID, marketID uuid.UUID, limit, offset int) ([]domain.Contact, int, error)
	// ListByGroup returns the contacts that are members of a contact group.
	ListByGroup(ctx context.Context, userID, groupID uuid.UUID, limit, offset int) ([]domain.Contact, int, error)
	// IDsForBusinesses maps the user's businesses to their contact ids (only
	// businesses that already have a contact are returned).
	IDsForBusinesses(ctx context.Context, userID uuid.UUID, businessIDs []uuid.UUID) ([]uuid.UUID, error)
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

// BulkEnsureItem identifies a business across the three result buckets so
// the UI can show recognisable names alongside the counts.
type BulkEnsureItem struct {
	BusinessID uuid.UUID `json:"business_id"`
	Name       string    `json:"name"`
}

// BulkEnsureResult breaks the input list into actionable buckets:
//   Added          → contacts created on this call
//   AlreadyExisted → contacts that already existed (idempotent skip)
//   NoEmail        → businesses with no email yet — surfaced so the user
//                    can manually research and enter one, then re-run
type BulkEnsureResult struct {
	Added          []BulkEnsureItem `json:"added"`
	AlreadyExisted []BulkEnsureItem `json:"already_existed"`
	NoEmail        []BulkEnsureItem `json:"no_email"`
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

// EnsureBulkFromBusinesses fans out across the input list, bucketing each
// business by outcome. Skips businesses with no email so the user can fix
// that up first (manual entry on the lead drawer); keeps the call
// idempotent so re-running after fixing emails just promotes them from
// no_email → added without disturbing existing contacts.
func (r *repository) EnsureBulkFromBusinesses(ctx context.Context, userID uuid.UUID, businessIDs []uuid.UUID) (BulkEnsureResult, error) {
	out := BulkEnsureResult{
		Added:          []BulkEnsureItem{},
		AlreadyExisted: []BulkEnsureItem{},
		NoEmail:        []BulkEnsureItem{},
	}
	if len(businessIDs) == 0 {
		return out, nil
	}

	// One read pass to grab name + email for every requested business.
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, email FROM businesses WHERE id = ANY($1)`,
		businessIDs,
	)
	if err != nil {
		return out, fmt.Errorf("load businesses for bulk ensure: %w", err)
	}
	type row struct {
		id    uuid.UUID
		name  string
		email *string
	}
	loaded := make(map[uuid.UUID]row, len(businessIDs))
	for rows.Next() {
		var rr row
		if err := rows.Scan(&rr.id, &rr.name, &rr.email); err != nil {
			rows.Close()
			return out, fmt.Errorf("scan bulk ensure row: %w", err)
		}
		loaded[rr.id] = rr
	}
	rows.Close()

	// One read pass for already-existing contacts.
	existRows, err := r.pool.Query(ctx,
		`SELECT business_id FROM contacts WHERE user_id = $1 AND business_id = ANY($2)`,
		userID, businessIDs,
	)
	if err != nil {
		return out, fmt.Errorf("load existing contacts: %w", err)
	}
	exists := make(map[uuid.UUID]bool, len(businessIDs))
	for existRows.Next() {
		var bid uuid.UUID
		if err := existRows.Scan(&bid); err != nil {
			existRows.Close()
			return out, err
		}
		exists[bid] = true
	}
	existRows.Close()

	// Bucket each requested ID. Insert in one transaction for atomicity.
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	for _, bid := range businessIDs {
		l, ok := loaded[bid]
		if !ok {
			// Unknown business id — skip silently rather than fail the
			// whole batch. (Could happen if a lead was deleted between
			// the user opening the page and clicking the button.)
			continue
		}
		if exists[bid] {
			out.AlreadyExisted = append(out.AlreadyExisted, BulkEnsureItem{BusinessID: bid, Name: l.name})
			continue
		}
		hasEmail := l.email != nil && *l.email != ""
		if !hasEmail {
			out.NoEmail = append(out.NoEmail, BulkEnsureItem{BusinessID: bid, Name: l.name})
			continue
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO contacts (id, user_id, business_id, primary_email,
			                       display_name, pipeline_stage, default_automation)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, 'lead', 'manual')
			 ON CONFLICT (user_id, business_id) DO NOTHING`,
			userID, bid, l.email, l.name,
		)
		if err != nil {
			return out, fmt.Errorf("bulk insert contact %s: %w", bid, err)
		}
		out.Added = append(out.Added, BulkEnsureItem{BusinessID: bid, Name: l.name})
	}

	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	return out, nil
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

func (r *repository) ListByMarket(ctx context.Context, userID, marketID uuid.UUID, limit, offset int) ([]domain.Contact, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM contacts c
		 JOIN business_markets bm ON bm.business_id = c.business_id
		 WHERE c.user_id = $1 AND bm.market_id = $2`,
		userID, marketID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count contacts by market: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.user_id, c.business_id, c.primary_email, c.primary_phone, c.display_name,
		        c.pipeline_stage, c.default_automation, c.default_sequence_id,
		        c.unsubscribed_at, c.unsubscribe_reason, c.created_at, c.updated_at
		 FROM contacts c
		 JOIN business_markets bm ON bm.business_id = c.business_id
		 WHERE c.user_id = $1 AND bm.market_id = $2
		 ORDER BY c.updated_at DESC
		 LIMIT $3 OFFSET $4`,
		userID, marketID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list contacts by market: %w", err)
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

func (r *repository) ListByGroup(ctx context.Context, userID, groupID uuid.UUID, limit, offset int) ([]domain.Contact, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM contacts c
		 JOIN contact_group_members m ON m.contact_id = c.id
		 WHERE c.user_id = $1 AND m.contact_group_id = $2`,
		userID, groupID,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count contacts by group: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.user_id, c.business_id, c.primary_email, c.primary_phone, c.display_name,
		        c.pipeline_stage, c.default_automation, c.default_sequence_id,
		        c.unsubscribed_at, c.unsubscribe_reason, c.created_at, c.updated_at
		 FROM contacts c
		 JOIN contact_group_members m ON m.contact_id = c.id
		 WHERE c.user_id = $1 AND m.contact_group_id = $2
		 ORDER BY c.updated_at DESC
		 LIMIT $3 OFFSET $4`,
		userID, groupID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list contacts by group: %w", err)
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

func (r *repository) IDsForBusinesses(ctx context.Context, userID uuid.UUID, businessIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(businessIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id FROM contacts WHERE user_id = $1 AND business_id = ANY($2)`,
		userID, businessIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("contact ids for businesses: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
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
