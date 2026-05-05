// Package campaign manages bulk outreach: a named batch of contacts with
// shared positioning, AI-drafted messages, per-row approval, and
// pace-controlled scheduled sending.
package campaign

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository interface {
	// Campaign CRUD --------------------------------------------------------
	Create(ctx context.Context, c domain.Campaign) (*domain.Campaign, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*domain.Campaign, error)
	GetUnscoped(ctx context.Context, id uuid.UUID) (*domain.Campaign, error)
	List(ctx context.Context, userID uuid.UUID) ([]domain.Campaign, error)
	Update(ctx context.Context, c domain.Campaign) (*domain.Campaign, error)
	UpdateStatus(ctx context.Context, userID, id uuid.UUID, status string, started, completed *time.Time) error
	Delete(ctx context.Context, userID, id uuid.UUID) error

	// Per-contact membership ----------------------------------------------
	AddContacts(ctx context.Context, campaignID uuid.UUID, contactIDs []uuid.UUID, marketID *uuid.UUID) (added int, err error)
	ListContacts(ctx context.Context, campaignID uuid.UUID, statusFilter string) ([]CampaignContactRow, error)
	GetContact(ctx context.Context, campaignID, contactID uuid.UUID) (*domain.CampaignContact, error)
	UpdateContactStatus(ctx context.Context, campaignID, contactID uuid.UUID, status string, draftMsgID *uuid.UUID, scheduledSendAt *time.Time) error
	UpdateContactSent(ctx context.Context, campaignID, contactID uuid.UUID, conversationID uuid.UUID, sentAt time.Time) error
	UpdateContactSkipped(ctx context.Context, campaignID, contactID uuid.UUID, reason string) error
	UpdateContactReplied(ctx context.Context, campaignID, contactID uuid.UUID, repliedAt time.Time) error

	// Worker queues -------------------------------------------------------
	NextPending(ctx context.Context, limit int) ([]CampaignContactWithCampaign, error)
	NextDueForSend(ctx context.Context, now time.Time, limit int) ([]CampaignContactWithCampaign, error)

	// Aggregates ----------------------------------------------------------
	Summary(ctx context.Context, userID, id uuid.UUID) (*domain.CampaignSummary, error)
	OverlapWith(ctx context.Context, userID uuid.UUID, contactIDs []uuid.UUID) ([]uuid.UUID, error)
}

// CampaignContactRow is the join used by the campaign detail page —
// includes the draft message preview when present.
type CampaignContactRow struct {
	domain.CampaignContact
	ContactName    string
	ContactEmail   *string
	BusinessName   string
	DraftSubject   *string
	DraftBodyText  *string
}

// CampaignContactWithCampaign is what the worker queues return — bundles
// the row with the campaign context the worker needs.
type CampaignContactWithCampaign struct {
	domain.CampaignContact
	Campaign domain.Campaign
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) Create(ctx context.Context, c domain.Campaign) (*domain.Campaign, error) {
	c.ID = uuid.New()
	if c.SendPacePerDay == 0 {
		c.SendPacePerDay = 50
	}
	if c.Status == "" {
		c.Status = domain.CampaignStatusDraft
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO campaigns (id, user_id, channel_id, name, goal, status,
		                         positioning_override, sequence_id, send_pace_per_day,
		                         attach_catalog, start_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		c.ID, c.UserID, c.ChannelID, c.Name, c.Goal, c.Status,
		c.PositioningOverride, c.SequenceID, c.SendPacePerDay,
		c.AttachCatalog, c.StartAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert campaign: %w", err)
	}
	return r.Get(ctx, c.UserID, c.ID)
}

func (r *repository) Get(ctx context.Context, userID, id uuid.UUID) (*domain.Campaign, error) {
	return r.scanCampaign(ctx,
		`SELECT id, user_id, channel_id, name, goal, status,
		        positioning_override, sequence_id, send_pace_per_day,
		        attach_catalog, start_at, started_at, completed_at,
		        created_at, updated_at
		 FROM campaigns WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
}

func (r *repository) GetUnscoped(ctx context.Context, id uuid.UUID) (*domain.Campaign, error) {
	return r.scanCampaign(ctx,
		`SELECT id, user_id, channel_id, name, goal, status,
		        positioning_override, sequence_id, send_pace_per_day,
		        attach_catalog, start_at, started_at, completed_at,
		        created_at, updated_at
		 FROM campaigns WHERE id = $1`,
		id,
	)
}

func (r *repository) scanCampaign(ctx context.Context, q string, args ...any) (*domain.Campaign, error) {
	var c domain.Campaign
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&c.ID, &c.UserID, &c.ChannelID, &c.Name, &c.Goal, &c.Status,
		&c.PositioningOverride, &c.SequenceID, &c.SendPacePerDay,
		&c.AttachCatalog, &c.StartAt, &c.StartedAt, &c.CompletedAt,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan campaign: %w", err)
	}
	return &c, nil
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]domain.Campaign, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, channel_id, name, goal, status,
		        positioning_override, sequence_id, send_pace_per_day,
		        attach_catalog, start_at, started_at, completed_at,
		        created_at, updated_at
		 FROM campaigns WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list campaigns: %w", err)
	}
	defer rows.Close()
	var out []domain.Campaign
	for rows.Next() {
		var c domain.Campaign
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.ChannelID, &c.Name, &c.Goal, &c.Status,
			&c.PositioningOverride, &c.SequenceID, &c.SendPacePerDay,
			&c.AttachCatalog, &c.StartAt, &c.StartedAt, &c.CompletedAt,
			&c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func (r *repository) Update(ctx context.Context, c domain.Campaign) (*domain.Campaign, error) {
	_, err := r.pool.Exec(ctx,
		`UPDATE campaigns SET
		   name = $1, goal = $2,
		   positioning_override = $3,
		   sequence_id = $4,
		   send_pace_per_day = $5,
		   attach_catalog = $6,
		   start_at = $7,
		   updated_at = now()
		 WHERE id = $8 AND user_id = $9`,
		c.Name, c.Goal, c.PositioningOverride, c.SequenceID,
		c.SendPacePerDay, c.AttachCatalog, c.StartAt,
		c.ID, c.UserID,
	)
	if err != nil {
		return nil, fmt.Errorf("update campaign: %w", err)
	}
	return r.Get(ctx, c.UserID, c.ID)
}

func (r *repository) UpdateStatus(ctx context.Context, userID, id uuid.UUID, status string, started, completed *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE campaigns SET
		   status = $1,
		   started_at = COALESCE($2, started_at),
		   completed_at = COALESCE($3, completed_at),
		   updated_at = now()
		 WHERE id = $4 AND user_id = $5`,
		status, started, completed, id, userID,
	)
	return err
}

func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM campaigns WHERE id = $1 AND user_id = $2 AND status = 'draft'`,
		id, userID,
	)
	return err
}

func (r *repository) AddContacts(ctx context.Context, campaignID uuid.UUID, contactIDs []uuid.UUID, marketID *uuid.UUID) (int, error) {
	if len(contactIDs) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	added := 0
	for _, cid := range contactIDs {
		tag, err := tx.Exec(ctx,
			`INSERT INTO campaign_contacts (campaign_id, contact_id, market_id, status)
			 VALUES ($1, $2, $3, 'pending')
			 ON CONFLICT (campaign_id, contact_id) DO NOTHING`,
			campaignID, cid, marketID,
		)
		if err != nil {
			return 0, fmt.Errorf("insert campaign_contact: %w", err)
		}
		added += int(tag.RowsAffected())
	}
	return added, tx.Commit(ctx)
}

func (r *repository) ListContacts(ctx context.Context, campaignID uuid.UUID, statusFilter string) ([]CampaignContactRow, error) {
	args := []any{campaignID}
	q := `SELECT cc.campaign_id, cc.contact_id, cc.market_id, cc.status,
	             cc.draft_message_id, cc.conversation_id, cc.scheduled_send_at,
	             cc.sent_at, cc.replied_at, cc.skip_reason, cc.added_at,
	             c.display_name, c.primary_email, b.name,
	             m.subject, m.body_text
	      FROM campaign_contacts cc
	      JOIN contacts c ON c.id = cc.contact_id
	      JOIN businesses b ON b.id = c.business_id
	      LEFT JOIN messages m ON m.id = cc.draft_message_id
	      WHERE cc.campaign_id = $1`
	if statusFilter != "" {
		q += " AND cc.status = $2"
		args = append(args, statusFilter)
	}
	q += " ORDER BY cc.added_at"
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list campaign_contacts: %w", err)
	}
	defer rows.Close()
	var out []CampaignContactRow
	for rows.Next() {
		var row CampaignContactRow
		if err := rows.Scan(
			&row.CampaignID, &row.ContactID, &row.MarketID, &row.Status,
			&row.DraftMessageID, &row.ConversationID, &row.ScheduledSendAt,
			&row.SentAt, &row.RepliedAt, &row.SkipReason, &row.AddedAt,
			&row.ContactName, &row.ContactEmail, &row.BusinessName,
			&row.DraftSubject, &row.DraftBodyText,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *repository) GetContact(ctx context.Context, campaignID, contactID uuid.UUID) (*domain.CampaignContact, error) {
	var cc domain.CampaignContact
	err := r.pool.QueryRow(ctx,
		`SELECT campaign_id, contact_id, market_id, status, draft_message_id,
		        conversation_id, scheduled_send_at, sent_at, replied_at, skip_reason, added_at
		 FROM campaign_contacts WHERE campaign_id = $1 AND contact_id = $2`,
		campaignID, contactID,
	).Scan(
		&cc.CampaignID, &cc.ContactID, &cc.MarketID, &cc.Status, &cc.DraftMessageID,
		&cc.ConversationID, &cc.ScheduledSendAt, &cc.SentAt, &cc.RepliedAt, &cc.SkipReason, &cc.AddedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan campaign_contact: %w", err)
	}
	return &cc, nil
}

func (r *repository) UpdateContactStatus(ctx context.Context, campaignID, contactID uuid.UUID, status string, draftMsgID *uuid.UUID, scheduledSendAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE campaign_contacts SET
		   status = $1,
		   draft_message_id = COALESCE($2, draft_message_id),
		   scheduled_send_at = COALESCE($3, scheduled_send_at)
		 WHERE campaign_id = $4 AND contact_id = $5`,
		status, draftMsgID, scheduledSendAt, campaignID, contactID,
	)
	return err
}

func (r *repository) UpdateContactSent(ctx context.Context, campaignID, contactID uuid.UUID, conversationID uuid.UUID, sentAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE campaign_contacts SET status = 'sent', conversation_id = $1, sent_at = $2
		 WHERE campaign_id = $3 AND contact_id = $4`,
		conversationID, sentAt, campaignID, contactID,
	)
	return err
}

func (r *repository) UpdateContactSkipped(ctx context.Context, campaignID, contactID uuid.UUID, reason string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE campaign_contacts SET status = 'skipped', skip_reason = $1
		 WHERE campaign_id = $2 AND contact_id = $3`,
		reason, campaignID, contactID,
	)
	return err
}

func (r *repository) UpdateContactReplied(ctx context.Context, campaignID, contactID uuid.UUID, repliedAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE campaign_contacts SET status = 'replied', replied_at = $1
		 WHERE campaign_id = $2 AND contact_id = $3`,
		repliedAt, campaignID, contactID,
	)
	return err
}

func (r *repository) NextPending(ctx context.Context, limit int) ([]CampaignContactWithCampaign, error) {
	return r.queueQuery(ctx,
		`SELECT cc.campaign_id, cc.contact_id, cc.market_id, cc.status, cc.draft_message_id,
		        cc.conversation_id, cc.scheduled_send_at, cc.sent_at, cc.replied_at, cc.skip_reason, cc.added_at,
		        ca.id, ca.user_id, ca.channel_id, ca.name, ca.goal, ca.status,
		        ca.positioning_override, ca.sequence_id, ca.send_pace_per_day,
		        ca.attach_catalog, ca.start_at, ca.started_at, ca.completed_at, ca.created_at, ca.updated_at
		 FROM campaign_contacts cc
		 JOIN campaigns ca ON ca.id = cc.campaign_id
		 WHERE cc.status = 'pending' AND ca.status IN ('draft','ready','active')
		 ORDER BY cc.added_at
		 LIMIT $1`, limit)
}

func (r *repository) NextDueForSend(ctx context.Context, now time.Time, limit int) ([]CampaignContactWithCampaign, error) {
	return r.queueQuery(ctx,
		`SELECT cc.campaign_id, cc.contact_id, cc.market_id, cc.status, cc.draft_message_id,
		        cc.conversation_id, cc.scheduled_send_at, cc.sent_at, cc.replied_at, cc.skip_reason, cc.added_at,
		        ca.id, ca.user_id, ca.channel_id, ca.name, ca.goal, ca.status,
		        ca.positioning_override, ca.sequence_id, ca.send_pace_per_day,
		        ca.attach_catalog, ca.start_at, ca.started_at, ca.completed_at, ca.created_at, ca.updated_at
		 FROM campaign_contacts cc
		 JOIN campaigns ca ON ca.id = cc.campaign_id
		 WHERE cc.status = 'approved' AND cc.scheduled_send_at <= $1
		   AND ca.status = 'active'
		 ORDER BY cc.scheduled_send_at
		 LIMIT $2`, now, limit)
}

func (r *repository) queueQuery(ctx context.Context, q string, args ...any) ([]CampaignContactWithCampaign, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CampaignContactWithCampaign
	for rows.Next() {
		var x CampaignContactWithCampaign
		if err := rows.Scan(
			&x.CampaignID, &x.ContactID, &x.MarketID, &x.Status, &x.DraftMessageID,
			&x.ConversationID, &x.ScheduledSendAt, &x.SentAt, &x.RepliedAt, &x.SkipReason, &x.AddedAt,
			&x.Campaign.ID, &x.Campaign.UserID, &x.Campaign.ChannelID, &x.Campaign.Name, &x.Campaign.Goal, &x.Campaign.Status,
			&x.Campaign.PositioningOverride, &x.Campaign.SequenceID, &x.Campaign.SendPacePerDay,
			&x.Campaign.AttachCatalog, &x.Campaign.StartAt, &x.Campaign.StartedAt, &x.Campaign.CompletedAt,
			&x.Campaign.CreatedAt, &x.Campaign.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func (r *repository) Summary(ctx context.Context, userID, id uuid.UUID) (*domain.CampaignSummary, error) {
	c, err := r.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx,
		`SELECT status, count(*) FROM campaign_contacts WHERE campaign_id = $1 GROUP BY status`,
		id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := domain.CampaignSummary{Campaign: *c}
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		out.TotalCount += n
		switch status {
		case domain.CampaignContactPending:
			out.PendingCount = n
		case domain.CampaignContactDrafted:
			out.DraftedCount = n
		case domain.CampaignContactApproved:
			out.ApprovedCount = n
		case domain.CampaignContactSent:
			out.SentCount = n
		case domain.CampaignContactReplied:
			out.RepliedCount = n
		case domain.CampaignContactSkipped:
			out.SkippedCount = n
		case domain.CampaignContactFailed:
			out.FailedCount = n
		}
	}
	return &out, nil
}

func (r *repository) OverlapWith(ctx context.Context, userID uuid.UUID, contactIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(contactIDs) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT contact_id FROM conversations
		 WHERE user_id = $1 AND status = 'active' AND contact_id = ANY($2)`,
		userID, contactIDs,
	)
	if err != nil {
		return nil, err
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

// kept to avoid an unused import when the file is in flux during dev.
var _ = strings.TrimSpace
