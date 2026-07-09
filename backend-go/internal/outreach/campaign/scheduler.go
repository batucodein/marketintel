package campaign

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/internalsched"
)

// Scheduler is the Tickable component that picks up approved campaign_contacts
// whose scheduled_send_at has passed and dispatches them through the channel.
// It applies the compliance pipeline (suppression check + List-Unsubscribe
// header + footer) automatically — campaign sends ALWAYS go through this
// gate, never the bare conversation.SendMessage path.
type Scheduler struct {
	svc        *Service
	maxPerTick int
}

func NewScheduler(svc *Service) *Scheduler {
	return &Scheduler{svc: svc, maxPerTick: 50}
}

func (s *Scheduler) SetMaxPerTick(n int) { s.maxPerTick = n }

func (s *Scheduler) Tick(ctx context.Context) (internalsched.TickResult, error) {
	res := internalsched.TickResult{Component: "campaign_scheduler"}
	rows, err := s.svc.repo.NextDueForSend(ctx, time.Now().UTC(), s.maxPerTick)
	if err != nil {
		return res, err
	}
	for _, row := range rows {
		if err := s.sendOne(ctx, row); err != nil {
			res.Errors++
			slog.Warn("campaign scheduler: row failed",
				"campaign_id", row.CampaignID, "contact_id", row.ContactID, "error", err,
			)
			continue
		}
		res.Processed++
	}
	return res, nil
}

func (s *Scheduler) sendOne(ctx context.Context, row CampaignContactWithCampaign) error {
	camp := row.Campaign

	// Reload contact + check suppression at send time, not draft time.
	c, err := s.svc.contacts.Get(ctx, camp.UserID, row.ContactID)
	if err != nil {
		return fmt.Errorf("load contact: %w", err)
	}
	if compliance.IsSuppressed(c) {
		return s.svc.repo.UpdateContactSkipped(ctx, row.CampaignID, row.ContactID, "contact unsubscribed")
	}
	if c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		return s.svc.repo.UpdateContactSkipped(ctx, row.CampaignID, row.ContactID, "no email")
	}

	if row.DraftMessageID == nil {
		return s.svc.repo.UpdateContactSkipped(ctx, row.CampaignID, row.ContactID, "no draft")
	}
	draft, err := s.svc.conversation.GetMessage(ctx, *row.DraftMessageID)
	if err != nil {
		return fmt.Errorf("load draft: %w", err)
	}
	subject := ""
	body := ""
	if draft.Subject != nil {
		subject = *draft.Subject
	}
	if draft.BodyText != nil {
		body = *draft.BodyText
	}
	if subject == "" || body == "" {
		return s.svc.repo.UpdateContactSkipped(ctx, row.CampaignID, row.ContactID, "draft empty")
	}

	uc, err := s.svc.channels.Get(ctx, camp.UserID, camp.ChannelID)
	if err != nil {
		// Channel row gone/unreadable — mark the row failed with the reason
		// so the UI surfaces it instead of the scheduler retrying forever.
		err = fmt.Errorf("load channel: %w", err)
		s.markFailed(ctx, row, err)
		return err
	}
	ch, err := s.svc.registry.Build(ctx, *uc)
	if err != nil {
		// The account exists but can't build a sender (e.g. credentials were
		// never stored / can't decrypt). Permanent until the user reconnects
		// — surface it rather than silently retrying every tick.
		err = fmt.Errorf("sending account %s isn't fully connected — reconnect it in Settings → Email channels (%w)", uc.FromEmail, err)
		s.markFailed(ctx, row, err)
		return err
	}

	// Build unsubscribe URL + appended footer (CAN-SPAM compliance).
	unsubURL, err := compliance.BuildUnsubscribeURL(s.svc.publicAPIURL, *uc, c.ID, camp.ID)
	if err != nil {
		return fmt.Errorf("build unsubscribe url: %w", err)
	}
	sp, _ := s.svc.resolveBrand(ctx, &camp)
	addr := ""
	if sp != nil {
		addr = sp.PhysicalAddress
	}
	bodyWithFooter := body + compliance.FooterText(addr, unsubURL)

	sendReq := channel.SendRequest{
		To:             *c.PrimaryEmail,
		Subject:        subject,
		BodyText:       bodyWithFooter,
		UnsubscribeURL: unsubURL,
	}

	// Optional catalog attachment (from the campaign's brand).
	if camp.AttachCatalog {
		fname, mimeType, data, err := s.svc.CatalogForCampaign(ctx, &camp)
		if err == nil && len(data) > 0 {
			sendReq.Attachments = []channel.Attachment{{
				Filename: fname, MimeType: mimeType, Data: data,
			}}
		}
	}

	result, err := ch.Send(ctx, sendReq)
	if err != nil {
		// Persist the failure so the campaign UI can show it and the
		// scheduler stops retrying every tick. The user can decide what
		// to do (reconnect channel, manually retry later, etc.).
		s.markFailed(ctx, row, err)
		// If Google says the OAuth grant is gone, the channel is dead
		// until the user reconnects. Flip the row so the UI's channels
		// page surfaces "Disconnected — Reconnect" and the inbound
		// poller stops hammering Gmail with 400s.
		if isOAuthRevoked(err) {
			_, _ = s.svc.pool.Exec(ctx,
				`UPDATE user_channels SET enabled = false, updated_at = now() WHERE id = $1`,
				uc.ID,
			)
			slog.Warn("campaign scheduler: gmail token revoked, auto-disabled channel",
				"channel_id", uc.ID, "user_id", camp.UserID)
		}
		return fmt.Errorf("channel send: %w", err)
	}

	now := time.Now().UTC()

	// Promote the paused conversation row + the draft message to "sent".
	convID := uuid.Nil
	if row.ConversationID != nil {
		convID = *row.ConversationID
	}
	tx, err := s.svc.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx,
		`UPDATE conversations SET
		   external_thread_id = $1, subject = COALESCE(subject, $2),
		   status = 'active', last_message_at = $3, last_direction = 'out',
		   updated_at = now()
		 WHERE id = $4`,
		result.ExternalThreadID, subject, now, convID,
	); err != nil {
		return fmt.Errorf("activate conversation: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE messages SET
		   external_id = $1, status = $2, sent_at = $3
		 WHERE id = $4`,
		result.ExternalMessageID, domain.MessageStatusSent, now, *row.DraftMessageID,
	); err != nil {
		return fmt.Errorf("flip draft to sent: %w", err)
	}

	if _, err := tx.Exec(ctx,
		`UPDATE campaign_contacts SET status = 'sent', sent_at = $1
		 WHERE campaign_id = $2 AND contact_id = $3`,
		now, row.CampaignID, row.ContactID,
	); err != nil {
		return fmt.Errorf("flip campaign_contact to sent: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// Move the contact's pipeline_stage from lead → contacted on first send.
	if c.PipelineStage == domain.PipelineLead {
		c.PipelineStage = domain.PipelineContacted
		_, _ = s.svc.contacts.Update(ctx, *c)
	}

	// If the campaign has an attached sequence, kick off a follow-up run on
	// this freshly-active conversation. The engine takes over from here.
	if camp.SequenceID != nil && s.svc.sequenceStarter != nil && convID != uuid.Nil {
		if err := s.svc.sequenceStarter.StartCampaignRun(ctx, *camp.SequenceID, convID); err != nil {
			// Non-fatal — log but don't undo the send.
			slog.Warn("campaign scheduler: start sequence run failed",
				"campaign_id", row.CampaignID, "contact_id", row.ContactID, "error", err)
		}
	}
	return nil
}

// markFailed flips a queue row to failed and records why, so the group UI
// can show the reason (and offer Retry) and the scheduler stops burning
// ticks on a row that cannot succeed. Best-effort: if the DB write fails
// (e.g. transient outage) the row stays approved and is retried next tick.
func (s *Scheduler) markFailed(ctx context.Context, row CampaignContactWithCampaign, reason error) {
	_ = s.svc.repo.UpdateContactStatus(ctx, row.CampaignID, row.ContactID, domain.CampaignContactFailed, nil, nil)
	_, _ = s.svc.pool.Exec(ctx,
		`UPDATE campaign_contacts SET skip_reason = $1 WHERE campaign_id = $2 AND contact_id = $3`,
		truncErr(reason), row.CampaignID, row.ContactID,
	)
}

// truncErr returns a DB-safe, UI-friendly version of an error message.
// Long Google API errors (often 2-3 KB of HTML) are clipped so the
// campaign UI can render the reason without exploding the row.
func truncErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 500 {
		s = s[:497] + "..."
	}
	return s
}

// isOAuthRevoked detects the specific Google response that means the
// user's refresh token is permanently dead: "invalid_grant" with
// "Token has been expired or revoked" in the error body, OR the
// 7-day-test-app expiry signal "Bad Request". Either way the only
// remedy is a fresh OAuth flow — auto-disable the channel.
func isOAuthRevoked(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "invalid_grant") ||
		strings.Contains(msg, "Token has been expired or revoked") ||
		strings.Contains(msg, "oauth2: cannot fetch token")
}
