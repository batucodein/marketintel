package conversation

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
	// Conversation ops
	Create(ctx context.Context, c domain.Conversation) (*domain.Conversation, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*domain.Conversation, error)
	GetByThread(ctx context.Context, channelID uuid.UUID, threadID string) (*domain.Conversation, error)
	FindActiveByContact(ctx context.Context, userID, contactID uuid.UUID) (*domain.Conversation, error)
	ListInbox(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]ConversationListRow, int, error)
	UpdateLast(ctx context.Context, id uuid.UUID, direction string, at time.Time, unread bool) error
	UpdateStatus(ctx context.Context, userID, id uuid.UUID, status string) error
	UpdateAutomation(ctx context.Context, userID, id uuid.UUID, automation string) error
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	Delete(ctx context.Context, userID, id uuid.UUID) error

	// Message ops
	CreateMessage(ctx context.Context, m domain.Message) (*domain.Message, error)
	ListMessages(ctx context.Context, conversationID uuid.UUID) ([]domain.Message, error)
	FindByExternalID(ctx context.Context, externalID string) (*domain.Message, error)
	DeletePendingDrafts(ctx context.Context, conversationID uuid.UUID) error
}

// ConversationListRow is a row in the inbox list — joined with contact + business for display.
type ConversationListRow struct {
	domain.Conversation
	ContactName       string    `json:"contact_name"`
	ContactEmail      *string   `json:"contact_email"`
	BusinessID        uuid.UUID `json:"business_id"`
	BusinessName      string    `json:"business_name"`
	LastMessageSnippet *string  `json:"last_message_snippet"`
	MessageCount       int      `json:"message_count"`
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) Create(ctx context.Context, c domain.Conversation) (*domain.Conversation, error) {
	c.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO conversations (
			id, user_id, contact_id, channel_id, campaign_id, channel_type,
			subject, external_thread_id, automation, status, last_message_at, last_direction, unread
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		c.ID, c.UserID, c.ContactID, c.ChannelID, c.CampaignID, c.ChannelType,
		c.Subject, c.ExternalThreadID, c.Automation, c.Status, c.LastMessageAt, c.LastDirection, c.Unread,
	)
	if err != nil {
		return nil, fmt.Errorf("insert conversation: %w", err)
	}
	return r.Get(ctx, c.UserID, c.ID)
}

func (r *repository) Get(ctx context.Context, userID, id uuid.UUID) (*domain.Conversation, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, contact_id, channel_id, campaign_id, channel_type,
		        subject, external_thread_id, automation, status,
		        last_message_at, last_direction, unread, created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
}

func (r *repository) GetByThread(ctx context.Context, channelID uuid.UUID, threadID string) (*domain.Conversation, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, contact_id, channel_id, campaign_id, channel_type,
		        subject, external_thread_id, automation, status,
		        last_message_at, last_direction, unread, created_at, updated_at
		 FROM conversations WHERE channel_id = $1 AND external_thread_id = $2`,
		channelID, threadID,
	)
}

func (r *repository) scanOne(ctx context.Context, q string, args ...any) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&c.ID, &c.UserID, &c.ContactID, &c.ChannelID, &c.CampaignID, &c.ChannelType,
		&c.Subject, &c.ExternalThreadID, &c.Automation, &c.Status,
		&c.LastMessageAt, &c.LastDirection, &c.Unread, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	return &c, nil
}

func (r *repository) ListInbox(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]ConversationListRow, int, error) {
	unreadFilter := ""
	args := []any{userID}
	if unreadOnly {
		unreadFilter = "AND c.unread = true"
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM conversations c WHERE c.user_id = $1 %s", unreadFilter),
		args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count inbox: %w", err)
	}

	args = append(args, limit, offset)
	limitIdx, offsetIdx := len(args)-1, len(args)

	q := fmt.Sprintf(
		`SELECT c.id, c.user_id, c.contact_id, c.channel_id, c.campaign_id, c.channel_type,
		        c.subject, c.external_thread_id, c.automation, c.status,
		        c.last_message_at, c.last_direction, c.unread, c.created_at, c.updated_at,
		        ct.display_name, ct.primary_email, b.id, b.name,
		        (SELECT substring(COALESCE(m.body_text, m.body_html, ''), 1, 140)
		         FROM messages m WHERE m.conversation_id = c.id
		         ORDER BY m.created_at DESC LIMIT 1) as snippet,
		        (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id) as msg_count
		 FROM conversations c
		 JOIN contacts ct ON ct.id = c.contact_id
		 JOIN businesses b ON b.id = ct.business_id
		 WHERE c.user_id = $1 %s
		 ORDER BY c.unread DESC, c.last_message_at DESC NULLS LAST
		 LIMIT $%d OFFSET $%d`,
		unreadFilter, limitIdx, offsetIdx,
	)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query inbox: %w", err)
	}
	defer rows.Close()

	var out []ConversationListRow
	for rows.Next() {
		var row ConversationListRow
		if err := rows.Scan(
			&row.ID, &row.UserID, &row.ContactID, &row.ChannelID, &row.CampaignID, &row.ChannelType,
			&row.Subject, &row.ExternalThreadID, &row.Automation, &row.Status,
			&row.LastMessageAt, &row.LastDirection, &row.Unread, &row.CreatedAt, &row.UpdatedAt,
			&row.ContactName, &row.ContactEmail, &row.BusinessID, &row.BusinessName,
			&row.LastMessageSnippet, &row.MessageCount,
		); err != nil {
			return nil, 0, fmt.Errorf("scan inbox row: %w", err)
		}
		out = append(out, row)
	}
	return out, total, nil
}

func (r *repository) UpdateLast(ctx context.Context, id uuid.UUID, direction string, at time.Time, unread bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversations SET last_message_at = $1, last_direction = $2, unread = $3, updated_at = now() WHERE id = $4`,
		at, direction, unread, id,
	)
	return err
}

func (r *repository) UpdateStatus(ctx context.Context, userID, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversations SET status = $1, updated_at = now() WHERE user_id = $2 AND id = $3`,
		status, userID, id,
	)
	return err
}

func (r *repository) UpdateAutomation(ctx context.Context, userID, id uuid.UUID, automation string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversations SET automation = $1, updated_at = now() WHERE user_id = $2 AND id = $3`,
		automation, userID, id,
	)
	return err
}

func (r *repository) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE conversations SET unread = false, updated_at = now() WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	return err
}

// FindActiveByContact returns the most recently active conversation for a contact,
// or ErrNotFound if none exists in status=active.
func (r *repository) FindActiveByContact(ctx context.Context, userID, contactID uuid.UUID) (*domain.Conversation, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, contact_id, channel_id, campaign_id, channel_type,
		        subject, external_thread_id, automation, status,
		        last_message_at, last_direction, unread, created_at, updated_at
		 FROM conversations
		 WHERE user_id = $1 AND contact_id = $2 AND status = 'active'
		 ORDER BY COALESCE(last_message_at, created_at) DESC
		 LIMIT 1`,
		userID, contactID,
	)
}

func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM conversations WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	return err
}

// DeletePendingDrafts removes any messages with status='pending_approval'
// in a conversation. Called before regenerating an AI draft so we don't
// leave stale drafts hanging around.
func (r *repository) DeletePendingDrafts(ctx context.Context, conversationID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM messages WHERE conversation_id = $1 AND status = 'pending_approval'`,
		conversationID,
	)
	return err
}

// --- Messages ----------------------------------------------------------

func (r *repository) CreateMessage(ctx context.Context, m domain.Message) (*domain.Message, error) {
	m.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO messages (
			id, conversation_id, direction, channel_type, external_id, in_reply_to_external_id,
			subject, body_text, body_html, ai_generated, ai_model, ai_prompt_version,
			status, campaign_id, sequence_step_id, sent_at, received_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		m.ID, m.ConversationID, m.Direction, m.ChannelType, m.ExternalID, m.InReplyToExternalID,
		m.Subject, m.BodyText, m.BodyHTML, m.AIGenerated, m.AIModel, m.AIPromptVersion,
		m.Status, m.CampaignID, m.SequenceStepID, m.SentAt, m.ReceivedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	return &m, nil
}

func (r *repository) ListMessages(ctx context.Context, conversationID uuid.UUID) ([]domain.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, conversation_id, direction, channel_type, external_id, in_reply_to_external_id,
		        subject, body_text, body_html, ai_generated, ai_model, ai_prompt_version,
		        status, campaign_id, sequence_step_id, sent_at, received_at, created_at
		 FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var out []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(
			&m.ID, &m.ConversationID, &m.Direction, &m.ChannelType, &m.ExternalID, &m.InReplyToExternalID,
			&m.Subject, &m.BodyText, &m.BodyHTML, &m.AIGenerated, &m.AIModel, &m.AIPromptVersion,
			&m.Status, &m.CampaignID, &m.SequenceStepID, &m.SentAt, &m.ReceivedAt, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *repository) FindByExternalID(ctx context.Context, externalID string) (*domain.Message, error) {
	var m domain.Message
	err := r.pool.QueryRow(ctx,
		`SELECT id, conversation_id, direction, channel_type, external_id, in_reply_to_external_id,
		        subject, body_text, body_html, ai_generated, ai_model, ai_prompt_version,
		        status, campaign_id, sequence_step_id, sent_at, received_at, created_at
		 FROM messages WHERE external_id = $1 LIMIT 1`, externalID,
	).Scan(
		&m.ID, &m.ConversationID, &m.Direction, &m.ChannelType, &m.ExternalID, &m.InReplyToExternalID,
		&m.Subject, &m.BodyText, &m.BodyHTML, &m.AIGenerated, &m.AIModel, &m.AIPromptVersion,
		&m.Status, &m.CampaignID, &m.SequenceStepID, &m.SentAt, &m.ReceivedAt, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan message: %w", err)
	}
	return &m, nil
}
