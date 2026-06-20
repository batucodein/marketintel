// Package poller pulls new inbound messages from every enabled user channel
// at a fixed interval and persists them as inbound messages on the matching
// conversations. Currently supports Gmail; other channels work via the same
// Channel interface.
package poller

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	netmail "net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/events"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// Poller walks every enabled user_channels row and pulls new messages.
// Safe to run a single instance; on multi-instance we'd need a lease.
type Poller struct {
	channels      channel.Repository
	registry      *channel.Registry
	conversations conversation.Repository
	contacts      contact.Repository
	broker        *events.Broker
	ai            *ai.Router
	pool          *pgxpool.Pool
	interval      time.Duration
}

func NewPoller(
	channels channel.Repository,
	registry *channel.Registry,
	conversations conversation.Repository,
	contacts contact.Repository,
	broker *events.Broker,
	aiRouter *ai.Router,
	pool *pgxpool.Pool,
	interval time.Duration,
) *Poller {
	if interval == 0 {
		interval = 2 * time.Minute
	}
	return &Poller{
		channels: channels, registry: registry,
		conversations: conversations, contacts: contacts,
		broker: broker, ai: aiRouter,
		pool: pool, interval: interval,
	}
}

// Run blocks until ctx is cancelled. Spawn this from main() in a goroutine.
// Deprecated: with Cloud Scheduler driving /internal/scheduler/tick, the
// scheduler calls PollOnce directly each tick. Kept here for local-dev use
// (set OUTREACH_POLLER_LOCAL=true to spawn it).
func (p *Poller) Run(ctx context.Context) {
	slog.Info("outreach poller started", "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// First tick immediately (useful in dev).
	p.PollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			slog.Info("outreach poller stopped")
			return
		case <-ticker.C:
			p.PollOnce(ctx)
		}
	}
}

// PollOnce runs a single inbound poll across every enabled channel.
// Safe to call from the scheduler tick handler; logs errors per-channel
// rather than aborting the whole pass.
func (p *Poller) PollOnce(ctx context.Context) {
	p.pollAll(ctx)
	p.reclassifyPending(ctx)
}

func (p *Poller) pollAll(ctx context.Context) {
	channels, err := p.channels.ListEnabled(ctx)
	if err != nil {
		slog.Warn("poller: list channels failed", "error", err)
		return
	}
	for _, uc := range channels {
		p.pollOne(ctx, uc)
	}
}

func (p *Poller) pollOne(ctx context.Context, uc domain.UserChannel) {
	// Determine "since". Default: 1 hour back on first poll, otherwise last_poll_at - 2 min overlap.
	since := time.Now().Add(-1 * time.Hour)
	if uc.LastPollAt != nil {
		since = uc.LastPollAt.Add(-2 * time.Minute) // small overlap to not miss boundary messages
	}

	ch, err := p.registry.Build(ctx, uc)
	if err != nil {
		slog.Warn("poller: build channel failed", "channel_id", uc.ID, "error", err)
		return
	}
	msgs, err := ch.ListNewMessages(ctx, since)
	if err != nil {
		slog.Warn("poller: list messages failed", "channel_id", uc.ID, "error", err)
		if isOAuthRevoked(err) {
			if _, dErr := p.pool.Exec(ctx,
				`UPDATE user_channels SET enabled = false, updated_at = now() WHERE id = $1`, uc.ID,
			); dErr == nil {
				slog.Warn("poller: gmail token revoked, auto-disabled channel",
					"channel_id", uc.ID, "user_id", uc.UserID)
			}
		}
		return
	}
	if len(msgs) == 0 {
		_ = p.channels.UpdateLastPoll(ctx, uc.ID, time.Now())
		return
	}

	inserted := 0
	for _, m := range msgs {
		if p.persistIncoming(ctx, uc, m) {
			inserted++
		}
	}
	_ = p.channels.UpdateLastPoll(ctx, uc.ID, time.Now())
	slog.Info("poller: pulled messages",
		"channel_id", uc.ID,
		"from_email", uc.FromEmail,
		"candidates", len(msgs),
		"inserted", inserted,
	)
}

// persistIncoming saves an inbound message if we haven't seen it before AND
// we can link it to an existing conversation. Messages from strangers are
// ignored in P1 — handling them needs UX design (auto-create contact? just drop?).
//
// Ordering matters: the message is classified BEFORE the conversation's
// last_direction flips to inbound, so the engine never branches on a reply
// whose sentiment/tags haven't been committed yet.
func (p *Poller) persistIncoming(ctx context.Context, uc domain.UserChannel, m channel.IncomingMessage) bool {
	if m.ExternalID == "" {
		return false
	}
	// Idempotency: skip if this external_id is already stored. On a lookup
	// error, bail rather than risk inserting a duplicate with all its side
	// effects (double classification, double replied-marking).
	existing, err := p.conversations.FindByExternalID(ctx, m.ExternalID)
	if err != nil {
		slog.Warn("poller: dedupe lookup failed, skipping message", "external_id", m.ExternalID, "error", err)
		return false
	}
	if existing != nil {
		return false
	}

	// Match to an existing conversation via a ladder, so we don't drop replies
	// that carry no provider thread id (IMAP has none) or that the buyer sent
	// "from scratch" instead of replying in-thread:
	//   1) provider thread id (Gmail),
	//   2) In-Reply-To header → our sent message's Message-ID → its conversation,
	//   3) sender address → that user's active conversation with the contact.
	// A genuine stranger (no match) is still ignored.
	var conv *domain.Conversation
	if m.ExternalThreadID != "" {
		conv, _ = p.conversations.GetByThread(ctx, uc.ID, m.ExternalThreadID)
	}
	if conv == nil && m.InReplyToExternalID != "" {
		if sent, ferr := p.conversations.FindByExternalID(ctx, m.InReplyToExternalID); ferr == nil && sent != nil {
			conv, _ = p.conversations.Get(ctx, uc.UserID, sent.ConversationID)
		}
	}
	if conv == nil {
		if from := parseEmailAddress(m.From); from != "" {
			conv, _ = p.conversations.FindActiveByContactEmail(ctx, uc.UserID, from)
		}
	}
	if conv == nil {
		// Not a reply to something we started, and not from a known contact — ignore.
		return false
	}

	subject := m.Subject
	body := m.BodyText
	htmlBody := m.BodyHTML

	now := m.ReceivedAt
	if now.IsZero() {
		now = time.Now()
	}

	msgID := uuid.New()
	_, err = p.conversations.CreateMessage(ctx, domain.Message{
		ID:                  msgID,
		ConversationID:      conv.ID,
		Direction:           domain.DirectionInbound,
		ChannelType:         conv.ChannelType,
		ExternalID:          strPtrIfNotEmpty(m.ExternalID),
		InReplyToExternalID: strPtrIfNotEmpty(m.InReplyToExternalID),
		Subject:             strPtrIfNotEmpty(subject),
		BodyText:            strPtrIfNotEmpty(body),
		BodyHTML:            strPtrIfNotEmpty(htmlBody),
		Status:              domain.MessageStatusSent,
		ReceivedAt:          &now,
	})
	if err != nil {
		slog.Warn("poller: insert inbound failed", "error", err)
		return false
	}

	// Classify ONLY the new text of the reply — quoted history contains our own
	// footer (with the word "Unsubscribe") and the original pitch, which would
	// contaminate sentiment, tags, and the opt-out keyword scan.
	newText := compliance.StripQuotedReply(body)

	score, label, legacy, tags, classified := p.classifyReply(ctx, uc.UserID, subject, newText)
	if classified {
		if err := p.conversations.SetInboundClassification(ctx, conv.ID, msgID, score, label, legacy, tags); err != nil {
			slog.Warn("poller: persist classification failed", "conversation_id", conv.ID, "error", err)
		}
	} else {
		// Leave the message's sentiment NULL — reclassifyPending retries it on a
		// later tick. Never fabricate a neutral classification from a failure.
		slog.Warn("poller: classification failed, will retry", "conversation_id", conv.ID, "message_id", msgID)
	}

	isOptOut := compliance.LooksLikeOptOut(newText)
	isBot := false
	for _, t := range tags {
		if t.Tag == domain.TagUnsubscribe {
			isOptOut = true
		}
		if t.Tag == domain.TagOutOfOffice {
			isBot = true
		}
	}

	c, cErr := p.contacts.Get(ctx, uc.UserID, conv.ContactID)

	switch {
	case isOptOut:
		// Compliance first: suppress immediately at ingestion (don't depend on a
		// sequence run being active for the engine to do it), and mark the
		// campaign row cold rather than "replied".
		if c != nil && c.UnsubscribedAt == nil {
			if err := p.contacts.MarkUnsubscribed(ctx, c.ID, "reply_optout"); err == nil {
				slog.Info("poller: contact unsubscribed via reply", "contact_id", c.ID)
			}
		}
		_, _ = p.pool.Exec(ctx,
			`UPDATE campaign_contacts SET status='cold' WHERE conversation_id=$1 AND status NOT IN ('cold','skipped','failed')`,
			conv.ID,
		)
	case isBot:
		// An auto-responder is not a human reply: don't mark the contact
		// "Replied" or advance the pipeline.
	default:
		// Mark the campaign_contact replied (first non-terminal reply) so the
		// Email Group "Replied" buckets reflect it.
		_, _ = p.pool.Exec(ctx,
			`UPDATE campaign_contacts SET status='replied', replied_at=now()
			 WHERE conversation_id=$1 AND status NOT IN ('cold','replied','skipped','failed')`,
			conv.ID,
		)
		// If contact was just "contacted", move to "replied".
		if cErr == nil && c != nil && c.PipelineStage == domain.PipelineContacted {
			c.PipelineStage = domain.PipelineReplied
			_, _ = p.contacts.Update(ctx, *c)
		}
	}

	// Only now flip the conversation to inbound (classification is committed,
	// so the engine branches on fresh data) and wake the follow-up run.
	_ = p.conversations.UpdateLast(ctx, conv.ID, domain.DirectionInbound, now, true)
	_, _ = p.pool.Exec(ctx,
		`UPDATE sequence_runs SET next_run_at=now() WHERE conversation_id=$1 AND status='active'`,
		conv.ID,
	)

	// Push to any open SSE subscribers so the inbox / conversation page
	// updates without a page refresh.
	if p.broker != nil {
		convID := conv.ID
		p.broker.Publish(events.Event{
			Kind:           events.KindInbound,
			UserID:         uc.UserID,
			ConversationID: &convID,
		})
	}
	return true
}

// classifyReply runs the classifier on an inbound reply's NEW text and returns
// the sentiment score, derived 5-level label, the legacy 3-value string (kept
// for the reply branch), validated intent tags, and whether classification
// actually succeeded. classified=false means infrastructure failure (AI down,
// bad JSON) — the caller must NOT persist a fabricated neutral.
func (p *Poller) classifyReply(ctx context.Context, userID uuid.UUID, subject, body string) (float64, string, string, []conversation.TagInput, bool) {
	if strings.TrimSpace(body) == "" {
		// No new text (pure-quote forward / empty auto-ack): genuinely neutral.
		return 0, domain.SentimentNeutral, domain.SentimentNeutral, nil, true
	}
	if p.ai == nil {
		slog.Warn("poller: AI router not configured, reply left unclassified")
		return 0, "", "", nil, false
	}
	prompt := prompts.BuildOutreachSentimentPrompt(prompts.OutreachSentimentInput{Subject: subject, Body: body})
	raw, _, err := p.ai.CompleteJSON(ai.WithUserID(ctx, userID), "outreach_sentiment", prompt.Prompt, prompt.System, 24*time.Hour)
	if err != nil {
		slog.Warn("poller: sentiment classification call failed", "error", err)
		return 0, "", "", nil, false
	}
	var out prompts.OutreachSentimentResult
	if err := json.Unmarshal(raw, &out); err != nil {
		slog.Warn("poller: sentiment classification returned invalid JSON", "error", err)
		return 0, "", "", nil, false
	}
	var score float64
	if out.Score != nil {
		score = clampScore(*out.Score)
	} else {
		// Model omitted the score field — fall back to its legacy string.
		score = domain.ScoreForLegacy(out.Sentiment)
	}
	label := domain.LabelForScore(score)
	legacy := domain.LegacyForScore(score)

	// Validate tags against the controlled vocabulary + a confidence floor — no
	// off-list or low-confidence (hallucinated) intent is persisted.
	var tags []conversation.TagInput
	seen := map[string]bool{}
	for _, t := range out.IntentTags {
		if !domain.IsValidIntentTag(t.Tag) || t.Confidence < 0.5 || seen[t.Tag] {
			continue
		}
		seen[t.Tag] = true
		tags = append(tags, conversation.TagInput{Tag: t.Tag, Confidence: t.Confidence})
	}
	return score, label, legacy, tags, true
}

func clampScore(s float64) float64 {
	switch {
	case s < -1:
		return -1
	case s > 1:
		return 1
	default:
		return s
	}
}

// reclassifyPending retries classification for recent inbound messages whose
// sentiment is still NULL (an earlier classify failed). Runs each poll tick;
// bounded so an extended AI outage can't pile up a huge burst afterwards.
func (p *Poller) reclassifyPending(ctx context.Context) {
	rows, err := p.pool.Query(ctx, `
		SELECT m.id, m.conversation_id, c.user_id, COALESCE(m.subject,''), COALESCE(m.body_text,'')
		  FROM messages m
		  JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.direction = 'in' AND m.sentiment IS NULL
		   AND COALESCE(m.body_text,'') <> ''
		   AND m.created_at > now() - interval '48 hours'
		 ORDER BY m.created_at
		 LIMIT 10`)
	if err != nil {
		return
	}
	defer rows.Close()
	type pending struct {
		msgID, convID, userID uuid.UUID
		subject, body         string
	}
	var todo []pending
	for rows.Next() {
		var it pending
		if err := rows.Scan(&it.msgID, &it.convID, &it.userID, &it.subject, &it.body); err == nil {
			todo = append(todo, it)
		}
	}
	rows.Close()
	for _, it := range todo {
		newText := compliance.StripQuotedReply(it.body)
		score, label, legacy, tags, ok := p.classifyReply(ctx, it.userID, it.subject, newText)
		if !ok {
			return // AI still down; try again next tick
		}
		if err := p.conversations.SetInboundClassification(ctx, it.convID, it.msgID, score, label, legacy, tags); err != nil {
			slog.Warn("poller: reclassify persist failed", "message_id", it.msgID, "error", err)
			continue
		}
		slog.Info("poller: reclassified pending inbound", "message_id", it.msgID, "label", label)
	}
}

// isOAuthRevoked mirrors the campaign-scheduler check — same predicate,
// kept private to each caller so we don't need a shared package just for
// one string match.
func isOAuthRevoked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "Token has been expired or revoked") ||
		strings.Contains(msg, "oauth2: cannot fetch token")
}

func strPtrIfNotEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// Summary is useful in tests / health endpoints to see what's configured.
func (p *Poller) Summary() string {
	return fmt.Sprintf("outreach poller (interval=%s)", p.interval)
}

// parseEmailAddress extracts the bare, lower-cased email from a From header
// value (which may be "Name <email@x>" or just "email@x"). Empty if none.
func parseEmailAddress(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	if addr, err := netmail.ParseAddress(from); err == nil {
		return strings.ToLower(addr.Address)
	}
	if i := strings.LastIndex(from, "<"); i >= 0 {
		if j := strings.LastIndex(from, ">"); j > i {
			return strings.ToLower(strings.TrimSpace(from[i+1 : j]))
		}
	}
	if strings.Contains(from, "@") {
		return strings.ToLower(from)
	}
	return ""
}
