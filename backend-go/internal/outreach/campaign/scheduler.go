package campaign

import (
	"context"
	"fmt"
	"log/slog"
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
		return fmt.Errorf("load channel: %w", err)
	}
	ch, err := s.svc.registry.Build(ctx, *uc)
	if err != nil {
		return fmt.Errorf("build channel: %w", err)
	}

	// Build unsubscribe URL + appended footer (CAN-SPAM compliance).
	unsubURL, err := compliance.BuildUnsubscribeURL(s.svc.publicAPIURL, *uc, c.ID, camp.ID)
	if err != nil {
		return fmt.Errorf("build unsubscribe url: %w", err)
	}
	sp, _ := s.svc.sender.Get(ctx, camp.UserID)
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

	// Optional catalog attachment.
	if camp.AttachCatalog {
		fname, mimeType, data, err := s.svc.sender.GetCatalogData(ctx, camp.UserID)
		if err == nil && len(data) > 0 {
			sendReq.Attachments = []channel.Attachment{{
				Filename: fname, MimeType: mimeType, Data: data,
			}}
		}
	}

	result, err := ch.Send(ctx, sendReq)
	if err != nil {
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
