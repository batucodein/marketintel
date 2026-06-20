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
	// FindActiveByContactEmail routes a from-scratch inbound (no thread id / no
	// In-Reply-To) back to its conversation by matching the sender address to a
	// contact's primary_email, scoped to the channel's user.
	FindActiveByContactEmail(ctx context.Context, userID uuid.UUID, email string) (*domain.Conversation, error)
	// ListByContact returns the contact's threads ordered by recency, each
	// with up to `messagesPerThread` of their latest sent/received messages
	// attached. Used by the campaign drafter and the conversation service
	// to give the AI "what's already been said with this contact" so it
	// doesn't write cold-opener boilerplate to a warm relationship.
	ListByContact(ctx context.Context, userID, contactID uuid.UUID, maxThreads, messagesPerThread int) ([]ThreadWithMessages, error)
	ListInbox(ctx context.Context, userID uuid.UUID, unreadOnly bool, limit, offset int) ([]ConversationListRow, int, error)
	UpdateLast(ctx context.Context, id uuid.UUID, direction string, at time.Time, unread bool) error
	// SetInboundClassification persists an inbound reply's sentiment score +
	// derived label (on the message and the conversation) and upserts its AI
	// intent tags. AI tags never clobber a manual tag. Legacy is the 3-value
	// string kept for back-compat with the reply branch.
	SetInboundClassification(ctx context.Context, convID, msgID uuid.UUID, score float64, label, legacy string, tags []TagInput) error
	// Intent tag ops (manual editing from the inbox / thread view).
	ListConvTags(ctx context.Context, conversationID uuid.UUID) ([]ConvTag, error)
	AddManualTag(ctx context.Context, conversationID uuid.UUID, tag string) error
	RemoveTag(ctx context.Context, conversationID uuid.UUID, tag string) error
	UpdateStatus(ctx context.Context, userID, id uuid.UUID, status string) error
	UpdateAutomation(ctx context.Context, userID, id uuid.UUID, automation string) error
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	Delete(ctx context.Context, userID, id uuid.UUID) error

	// Message ops
	CreateMessage(ctx context.Context, m domain.Message) (*domain.Message, error)
	GetMessage(ctx context.Context, id uuid.UUID) (*domain.Message, error)
	UpdateMessageBody(ctx context.Context, id uuid.UUID, body string) error
	UpdateMessageDraft(ctx context.Context, id uuid.UUID, subject, body string) error
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
	LastMessageSnippet        *string  `json:"last_message_snippet"`
	MessageCount              int      `json:"message_count"`
	LastInboundSentiment      *string  `json:"last_inbound_sentiment"`
	LastInboundSentimentScore *float64 `json:"last_inbound_sentiment_score"`
	LastInboundSentimentLabel *string  `json:"last_inbound_sentiment_label"`
	Tags                      []string `json:"tags"`
}

// ThreadWithMessages is the shape returned by ListByContact. Each entry
// is one prior conversation thread with its recent messages embedded so
// the caller can render them into a single prompt block without making
// N+1 queries.
type ThreadWithMessages struct {
	Conversation domain.Conversation `json:"conversation"`
	Messages     []domain.Message    `json:"messages"` // chronological, oldest first
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

	// Hide paused placeholder conversations created during campaign
	// drafting — they only become real (status=active) when the scheduler
	// actually fires the send. Otherwise the inbox shows scheduled rows
	// that look identical to sent ones.
	statusFilter := "AND c.status <> 'paused'"

	var total int
	if err := r.pool.QueryRow(ctx,
		fmt.Sprintf("SELECT count(*) FROM conversations c WHERE c.user_id = $1 %s %s", unreadFilter, statusFilter),
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
		        c.last_inbound_sentiment, c.last_inbound_sentiment_score, c.last_inbound_sentiment_label,
		        COALESCE(ARRAY(SELECT tag FROM conversation_tags WHERE conversation_id = c.id ORDER BY tag), '{}'),
		        ct.display_name, ct.primary_email, b.id, b.name,
		        (SELECT substring(COALESCE(m.body_text, m.body_html, ''), 1, 140)
		         FROM messages m WHERE m.conversation_id = c.id
		         ORDER BY m.created_at DESC LIMIT 1) as snippet,
		        (SELECT count(*) FROM messages m WHERE m.conversation_id = c.id) as msg_count
		 FROM conversations c
		 JOIN contacts ct ON ct.id = c.contact_id
		 JOIN businesses b ON b.id = ct.business_id
		 WHERE c.user_id = $1 %s %s
		 ORDER BY c.unread DESC, c.last_message_at DESC NULLS LAST
		 LIMIT $%d OFFSET $%d`,
		unreadFilter, statusFilter, limitIdx, offsetIdx,
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
			&row.LastInboundSentiment, &row.LastInboundSentimentScore, &row.LastInboundSentimentLabel,
			&row.Tags,
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

// TagInput is one intent tag to upsert for a conversation.
type TagInput struct {
	Tag        string
	Confidence float64
}

// ConvTag is a stored intent tag with its provenance.
type ConvTag struct {
	Tag        string   `json:"tag"`
	Source     string   `json:"source"`
	Confidence *float64 `json:"confidence"`
}

func (r *repository) ListConvTags(ctx context.Context, conversationID uuid.UUID) ([]ConvTag, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT tag, source, confidence FROM conversation_tags WHERE conversation_id = $1 ORDER BY tag`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConvTag{}
	for rows.Next() {
		var t ConvTag
		if err := rows.Scan(&t.Tag, &t.Source, &t.Confidence); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

// AddManualTag adds (or promotes) a tag as user-set. A manual tag overrides any
// existing ai-sourced row so the classifier won't later clobber it.
func (r *repository) AddManualTag(ctx context.Context, conversationID uuid.UUID, tag string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO conversation_tags (conversation_id, tag, source, confidence)
		 VALUES ($1, $2, 'manual', NULL)
		 ON CONFLICT (conversation_id, tag)
		 DO UPDATE SET source = 'manual', confidence = NULL`,
		conversationID, tag,
	)
	return err
}

func (r *repository) RemoveTag(ctx context.Context, conversationID uuid.UUID, tag string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM conversation_tags WHERE conversation_id = $1 AND tag = $2`,
		conversationID, tag,
	)
	return err
}

func (r *repository) SetInboundClassification(ctx context.Context, convID, msgID uuid.UUID, score float64, label, legacy string, tags []TagInput) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE messages SET sentiment = $1, sentiment_score = $2, sentiment_label = $3 WHERE id = $4`,
		legacy, score, label, msgID,
	); err != nil {
		return fmt.Errorf("update message sentiment: %w", err)
	}
	// Conversation-level fields describe the LATEST inbound reply — guard so a
	// delayed re-classification of an older message can't overwrite them.
	isLatest := false
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE((SELECT id FROM messages WHERE conversation_id=$1 AND direction='in' ORDER BY created_at DESC, id DESC LIMIT 1) = $2, false)`,
		convID, msgID,
	).Scan(&isLatest); err != nil {
		return fmt.Errorf("latest-inbound check: %w", err)
	}
	if isLatest {
		if _, err := tx.Exec(ctx,
			`UPDATE conversations
			   SET last_inbound_sentiment = $1, last_inbound_sentiment_score = $2, last_inbound_sentiment_label = $3
			 WHERE id = $4`,
			legacy, score, label, convID,
		); err != nil {
			return fmt.Errorf("update conversation sentiment: %w", err)
		}
		// Tags describe the current reply, not the conversation's lifetime —
		// replace the previous reply's AI tags (manual tags always survive).
		// Routing, guidance matching, and facets all key off the current state.
		if _, err := tx.Exec(ctx,
			`DELETE FROM conversation_tags WHERE conversation_id = $1 AND source = 'ai'`, convID,
		); err != nil {
			return fmt.Errorf("clear previous ai tags: %w", err)
		}
		for _, t := range tags {
			// Insert AI tags; never overwrite a manually-set tag.
			if _, err := tx.Exec(ctx,
				`INSERT INTO conversation_tags (conversation_id, tag, source, confidence)
				 VALUES ($1, $2, 'ai', $3)
				 ON CONFLICT (conversation_id, tag) DO NOTHING`,
				convID, t.Tag, t.Confidence,
			); err != nil {
				return fmt.Errorf("insert tag %q: %w", t.Tag, err)
			}
		}
	}
	return tx.Commit(ctx)
}

func (r *repository) UpdateMessageBody(ctx context.Context, id uuid.UUID, body string) error {
	// body_html is cleared: it could only be stale relative to the new text.
	_, err := r.pool.Exec(ctx, `UPDATE messages SET body_text=$1, body_html=NULL WHERE id=$2`, body, id)
	return err
}

// UpdateMessageDraft rewrites a draft's subject + body (cold-opener refine).
func (r *repository) UpdateMessageDraft(ctx context.Context, id uuid.UUID, subject, body string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE messages SET subject=$1, body_text=$2, body_html=NULL WHERE id=$3`, subject, body, id)
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

// FindActiveByContactEmail returns the most-recently-active conversation for the
// user whose contact has this primary_email (case-insensitive). ErrNotFound if none.
func (r *repository) FindActiveByContactEmail(ctx context.Context, userID uuid.UUID, email string) (*domain.Conversation, error) {
	return r.scanOne(ctx,
		`SELECT cv.id, cv.user_id, cv.contact_id, cv.channel_id, cv.campaign_id, cv.channel_type,
		        cv.subject, cv.external_thread_id, cv.automation, cv.status,
		        cv.last_message_at, cv.last_direction, cv.unread, cv.created_at, cv.updated_at
		 FROM conversations cv
		 JOIN contacts ct ON ct.id = cv.contact_id
		 WHERE cv.user_id = $1 AND lower(ct.primary_email) = lower($2) AND cv.status = 'active'
		 ORDER BY COALESCE(cv.last_message_at, cv.created_at) DESC
		 LIMIT 1`,
		userID, email,
	)
}

func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM conversations WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	return err
}

// ListByContact pulls every thread the user has ever had with this
// contact (any channel, any campaign), newest first, capped at maxThreads.
// For each thread we attach up to messagesPerThread of its most recent
// non-draft messages so the AI can see the actual content of past
// exchanges, not just a "this thread existed" header.
func (r *repository) ListByContact(ctx context.Context, userID, contactID uuid.UUID, maxThreads, messagesPerThread int) ([]ThreadWithMessages, error) {
	if maxThreads <= 0 {
		maxThreads = 5
	}
	if messagesPerThread <= 0 {
		messagesPerThread = 10
	}

	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, contact_id, channel_id, campaign_id, channel_type,
		        subject, external_thread_id, automation, status,
		        last_message_at, last_direction, unread, created_at, updated_at
		 FROM conversations
		 WHERE user_id = $1 AND contact_id = $2 AND status <> 'paused'
		 ORDER BY COALESCE(last_message_at, created_at) DESC
		 LIMIT $3`,
		userID, contactID, maxThreads,
	)
	if err != nil {
		return nil, fmt.Errorf("list threads by contact: %w", err)
	}
	defer rows.Close()

	var out []ThreadWithMessages
	for rows.Next() {
		var c domain.Conversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.ContactID, &c.ChannelID, &c.CampaignID, &c.ChannelType,
			&c.Subject, &c.ExternalThreadID, &c.Automation, &c.Status,
			&c.LastMessageAt, &c.LastDirection, &c.Unread, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan thread: %w", err)
		}
		out = append(out, ThreadWithMessages{Conversation: c})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Hydrate messages per thread. Two-step rather than one giant join
	// because we want a LIMIT per-thread, which Postgres handles cleanly
	// with a lateral subquery but is fiddly to express here.
	for i := range out {
		msgs, err := r.recentNonDraftMessages(ctx, out[i].Conversation.ID, messagesPerThread)
		if err != nil {
			// Skip this thread's body content rather than fail the whole
			// caller — partial context is better than no context.
			continue
		}
		out[i].Messages = msgs
	}
	return out, nil
}

// recentNonDraftMessages returns the latest N sent/received messages of
// a conversation in chronological order (oldest first), so the AI reads
// them in natural reading order.
func (r *repository) recentNonDraftMessages(ctx context.Context, conversationID uuid.UUID, limit int) ([]domain.Message, error) {
	rows, err := r.pool.Query(ctx,
		`WITH recent AS (
		   SELECT id, conversation_id, direction, channel_type, external_id, in_reply_to_external_id,
		          subject, body_text, body_html, ai_generated, ai_model, ai_prompt_version,
		          status, campaign_id, sequence_step_id, sent_at, received_at, created_at
		   FROM messages
		   WHERE conversation_id = $1 AND status NOT IN ('draft', 'pending_approval')
		   ORDER BY created_at DESC
		   LIMIT $2
		 )
		 SELECT * FROM recent ORDER BY created_at ASC`,
		conversationID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Message
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Direction, &m.ChannelType, &m.ExternalID, &m.InReplyToExternalID,
			&m.Subject, &m.BodyText, &m.BodyHTML, &m.AIGenerated, &m.AIModel, &m.AIPromptVersion,
			&m.Status, &m.CampaignID, &m.SequenceStepID, &m.SentAt, &m.ReceivedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
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

func (r *repository) GetMessage(ctx context.Context, id uuid.UUID) (*domain.Message, error) {
	var m domain.Message
	err := r.pool.QueryRow(ctx,
		`SELECT id, conversation_id, direction, channel_type, external_id, in_reply_to_external_id,
		        subject, body_text, body_html, ai_generated, ai_model, ai_prompt_version,
		        status, campaign_id, sequence_step_id, sent_at, received_at, created_at
		 FROM messages WHERE id = $1`, id,
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
