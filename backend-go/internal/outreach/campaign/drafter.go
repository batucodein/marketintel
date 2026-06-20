package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/internalsched"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// Drafter is the Tickable component that fills in pending campaign_contacts
// with AI-drafted messages. Each scheduler tick processes up to maxPerTick
// pending rows. Errors on individual rows are logged and the row stays
// pending so the next tick retries it.
type Drafter struct {
	svc        *Service
	maxPerTick int
}

func NewDrafter(svc *Service) *Drafter {
	return &Drafter{svc: svc, maxPerTick: 50}
}

// SetMaxPerTick overrides the default batch size (useful in tests).
func (d *Drafter) SetMaxPerTick(n int) { d.maxPerTick = n }

func (d *Drafter) Tick(ctx context.Context) (internalsched.TickResult, error) {
	res := internalsched.TickResult{Component: "campaign_drafter"}
	rows, err := d.svc.repo.NextPending(ctx, d.maxPerTick)
	if err != nil {
		return res, err
	}
	for _, row := range rows {
		if err := d.draftOne(ctx, row); err != nil {
			res.Errors++
			slog.Warn("campaign drafter: row failed",
				"campaign_id", row.CampaignID, "contact_id", row.ContactID, "error", err,
			)
			continue
		}
		res.Processed++
	}
	return res, nil
}

func (d *Drafter) draftOne(ctx context.Context, row CampaignContactWithCampaign) error {
	camp := row.Campaign
	c, err := d.svc.contacts.Get(ctx, camp.UserID, row.ContactID)
	if err != nil {
		return fmt.Errorf("load contact: %w", err)
	}
	if c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		return d.svc.repo.UpdateContactSkipped(ctx, row.CampaignID, row.ContactID, "no email on contact")
	}
	if c.UnsubscribedAt != nil {
		return d.svc.repo.UpdateContactSkipped(ctx, row.CampaignID, row.ContactID, "contact unsubscribed")
	}

	sp, _ := d.svc.resolveBrand(ctx, &camp)
	if sp == nil {
		sp = &domain.SenderProfile{UserID: camp.UserID, Tone: "formal"}
	}

	override := d.svc.parsePositioning(&camp)
	if override != nil {
		// Apply campaign overrides on top of the sender profile copy used
		// for prompting. Doesn't mutate the row in DB.
		if override.ProductDescription != "" {
			sp.ProductDescription = override.ProductDescription
		}
		if override.ValueProp != "" {
			sp.ValueProp = override.ValueProp
		}
		if override.Tone != "" {
			sp.Tone = override.Tone
		}
	}

	hasCatalog := camp.AttachCatalog
	catalogName := ""
	if hasCatalog {
		// Confirm a catalog actually exists; if not, downgrade silently.
		ok, name := senderCatalogIfPresent(sp)
		hasCatalog = ok
		catalogName = name
	}

	businessJSON := d.svc.leadCtx.BusinessJSON(ctx, c.BusinessID)
	shipmentCtx := d.svc.leadCtx.ShipmentContext(ctx, c.BusinessID)
	leadScoreJSON := d.svc.leadCtx.LeadScoreJSON(ctx, camp.UserID, c.BusinessID)

	positioning := ""
	if override != nil && override.Goal != "" {
		positioning = override.Goal
	} else if camp.Goal != "" {
		positioning = camp.Goal
	}

	// Pull prior conversations the user has had with this contact across
	// any campaign/channel. If any exist, the draft will be a warm
	// re-engage rather than a cold first touch.
	priorBlock := ""
	threads, terr := d.svc.conversation.ListByContact(ctx, camp.UserID, c.ID, 5, 10)
	if terr == nil && len(threads) > 0 {
		priorBlock = conversation.RenderPriorConversations(threads, time.Now())
	}

	in := prompts.OutreachDraftInput{
		SenderProfileJSON:         prompts.EncodeJSON(sp),
		CampaignPositioning:       positioning,
		ContactJSON:               prompts.EncodeJSON(c),
		BusinessJSON:              businessJSON,
		ShipmentContext:           shipmentCtx,
		LeadScoreJSON:             leadScoreJSON,
		PriorConversationsContext: priorBlock,
		HasCatalog:                hasCatalog,
		CatalogFilename:           catalogName,
	}
	out, err := d.draftWithRetry(ctx, camp.UserID, in)
	if err != nil {
		return err
	}
	// Guarantee the brand signature is present (the model is inconsistent).
	out.Body = prompts.WithSignature(out.Body, sp.Signature)

	// Pre-create a paused conversation so the FK on messages.conversation_id
	// resolves. The scheduler unpauses it when it actually sends.
	convID, err := d.preCreateConversation(ctx, camp, c)
	if err != nil {
		return fmt.Errorf("pre-create conversation: %w", err)
	}

	promptV := "outreach_draft_v1"
	msgID := uuid.New()
	if _, err = d.svc.pool.Exec(ctx,
		`INSERT INTO messages (id, conversation_id, direction, channel_type, subject,
		                       body_text, ai_generated, ai_prompt_version, status, campaign_id)
		 VALUES ($1, $2, $3, (SELECT channel_type FROM conversations WHERE id = $2), $4, $5, true, $6, $7, $8)`,
		msgID, convID, domain.DirectionOutbound,
		out.Subject, out.Body, promptV, domain.MessageStatusDraft, camp.ID,
	); err != nil {
		return fmt.Errorf("insert draft message: %w", err)
	}

	// Link conversation + draft back onto the campaign_contacts row.
	if _, err := d.svc.pool.Exec(ctx,
		`UPDATE campaign_contacts SET conversation_id = $1, draft_message_id = $2, status = 'drafted'
		 WHERE campaign_id = $3 AND contact_id = $4`,
		convID, msgID, row.CampaignID, row.ContactID,
	); err != nil {
		return fmt.Errorf("link draft to campaign_contact: %w", err)
	}
	return nil
}

// preCreateConversation creates a paused conversation row to satisfy the FK
// on messages.conversation_id. The scheduler activates it on send.
func (d *Drafter) preCreateConversation(ctx context.Context, camp domain.Campaign, c *domain.Contact) (uuid.UUID, error) {
	convID := uuid.New()
	_, err := d.svc.pool.Exec(ctx,
		`INSERT INTO conversations (id, user_id, contact_id, channel_id, campaign_id,
		                            channel_type, automation, status)
		 VALUES ($1, $2, $3, $4, $5,
		         COALESCE((SELECT type FROM user_channels WHERE id = $4), $6), 'manual', 'paused')`,
		convID, camp.UserID, c.ID, camp.ChannelID, camp.ID, domain.ChannelTypeGmailOAuth,
	)
	if err != nil {
		return uuid.Nil, err
	}
	return convID, nil
}

// draftWithRetry runs one AI completion, validates output with the
// prompt-side guard, and re-runs once with StricterRetry=true if the
// first pass tripped any forbidden-pattern or catalog-hedge violation.
// On retry failure we fall back to the first attempt rather than dropping
// the row entirely — a slightly off-brand email is better than no email.
func (d *Drafter) draftWithRetry(ctx context.Context, userID uuid.UUID, in prompts.OutreachDraftInput) (*prompts.OutreachDraftResult, error) {
	first, err := d.draftOnce(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	mode := prompts.DraftModeCold
	if in.PriorConversationsContext != "" {
		mode = prompts.DraftModeReply
	}
	violations := prompts.ValidateOutreachBody(first.Body, in.HasCatalog, mode)
	if len(violations) == 0 {
		return first, nil
	}
	slog.Warn("campaign drafter: guard violations, retrying",
		"violations", violations, "subject", first.Subject,
	)
	in.StricterRetry = true
	second, err := d.draftOnce(ctx, userID, in)
	if err != nil {
		return first, nil
	}
	return second, nil
}

func (d *Drafter) draftOnce(ctx context.Context, userID uuid.UUID, in prompts.OutreachDraftInput) (*prompts.OutreachDraftResult, error) {
	prompt := prompts.BuildOutreachDraftPrompt(in)
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := d.svc.ai.CompleteJSON(ctxAI, "outreach_draft", prompt.Prompt, prompt.System, 30*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("ai draft: %w", err)
	}
	var out prompts.OutreachDraftResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse draft JSON: %w", err)
	}
	if out.Subject == "" || out.Body == "" {
		return nil, fmt.Errorf("empty subject or body in draft")
	}
	return &out, nil
}

// senderCatalogIfPresent re-implements the same gate used by single-send.
// Confirms a catalog file is actually present on the sender profile —
// AttachCatalog=true on the campaign without an uploaded file silently
// downgrades to "no catalog" so the prompt doesn't promise an attachment
// that won't be there at send time.
func senderCatalogIfPresent(sp *domain.SenderProfile) (bool, string) {
	if sp == nil || sp.CatalogFileName == nil || *sp.CatalogFileName == "" {
		return false, ""
	}
	if sp.CatalogSizeBytes == nil || *sp.CatalogSizeBytes <= 0 {
		return false, ""
	}
	return true, *sp.CatalogFileName
}
