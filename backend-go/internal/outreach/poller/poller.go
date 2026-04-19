// Package poller pulls new inbound messages from every enabled user channel
// at a fixed interval and persists them as inbound messages on the matching
// conversations. Currently supports Gmail; other channels work via the same
// Channel interface.
package poller

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
)

// Poller walks every enabled user_channels row and pulls new messages.
// Safe to run a single instance; on multi-instance we'd need a lease.
type Poller struct {
	channels      channel.Repository
	registry      *channel.Registry
	conversations conversation.Repository
	contacts      contact.Repository
	pool          *pgxpool.Pool
	interval      time.Duration
}

func NewPoller(
	channels channel.Repository,
	registry *channel.Registry,
	conversations conversation.Repository,
	contacts contact.Repository,
	pool *pgxpool.Pool,
	interval time.Duration,
) *Poller {
	if interval == 0 {
		interval = 2 * time.Minute
	}
	return &Poller{
		channels: channels, registry: registry,
		conversations: conversations, contacts: contacts,
		pool: pool, interval: interval,
	}
}

// Run blocks until ctx is cancelled. Spawn this from main() in a goroutine.
func (p *Poller) Run(ctx context.Context) {
	slog.Info("outreach poller started", "interval", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	// First tick immediately (useful in dev).
	p.pollAll(ctx)
	for {
		select {
		case <-ctx.Done():
			slog.Info("outreach poller stopped")
			return
		case <-ticker.C:
			p.pollAll(ctx)
		}
	}
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
func (p *Poller) persistIncoming(ctx context.Context, uc domain.UserChannel, m channel.IncomingMessage) bool {
	if m.ExternalID == "" {
		return false
	}
	// Idempotency: skip if this external_id is already stored.
	existing, _ := p.conversations.FindByExternalID(ctx, m.ExternalID)
	if existing != nil {
		return false
	}

	// Match to an existing conversation by thread ID on this channel.
	conv, err := p.conversations.GetByThread(ctx, uc.ID, m.ExternalThreadID)
	if err != nil || conv == nil {
		// Not a reply to something we started — ignore for now.
		return false
	}

	subject := m.Subject
	body := m.BodyText
	htmlBody := m.BodyHTML

	now := m.ReceivedAt
	if now.IsZero() {
		now = time.Now()
	}

	_, err = p.conversations.CreateMessage(ctx, domain.Message{
		ID:                  uuid.New(),
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

	// Mark conversation as unread + bump last message.
	_ = p.conversations.UpdateLast(ctx, conv.ID, domain.DirectionInbound, now, true)

	// If contact was just "contacted", move to "replied".
	c, err := p.contacts.Get(ctx, uc.UserID, conv.ContactID)
	if err == nil && c != nil && c.PipelineStage == domain.PipelineContacted {
		c.PipelineStage = domain.PipelineReplied
		_, _ = p.contacts.Update(ctx, *c)
	}
	return true
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
