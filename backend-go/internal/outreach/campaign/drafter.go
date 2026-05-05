package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
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

	sp, _ := d.svc.sender.Get(ctx, camp.UserID)
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

	prompt := prompts.BuildOutreachDraftPrompt(prompts.OutreachDraftInput{
		SenderProfileJSON:   prompts.EncodeJSON(sp),
		CampaignPositioning: positioning,
		ContactJSON:         prompts.EncodeJSON(c),
		BusinessJSON:        businessJSON,
		ShipmentContext:     shipmentCtx,
		LeadScoreJSON:       leadScoreJSON,
		HasCatalog:          hasCatalog,
		CatalogFilename:     catalogName,
	})

	ctxAI := ai.WithUserID(ctx, camp.UserID)
	raw, _, err := d.svc.ai.CompleteJSON(ctxAI, "outreach_draft", prompt.Prompt, prompt.System, 30*time.Minute)
	if err != nil {
		return fmt.Errorf("ai draft: %w", err)
	}
	var out prompts.OutreachDraftResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("parse ai draft: %w", err)
	}
	if out.Subject == "" || out.Body == "" {
		return fmt.Errorf("ai returned empty subject/body")
	}

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
		 VALUES ($1, $2, $3, $4, $5, $6, true, $7, $8, $9)`,
		msgID, convID, domain.DirectionOutbound, domain.ChannelTypeGmailOAuth,
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
		 VALUES ($1, $2, $3, $4, $5, $6, 'manual', 'paused')`,
		convID, camp.UserID, c.ID, camp.ChannelID, camp.ID, domain.ChannelTypeGmailOAuth,
	)
	if err != nil {
		return uuid.Nil, err
	}
	return convID, nil
}

// senderCatalogIfPresent re-implements the same gate used by single-send so
// the campaign drafter doesn't need to import conversation pkg.
func senderCatalogIfPresent(sp *domain.SenderProfile) (bool, string) {
	if sp == nil || sp.CatalogFileName == nil || *sp.CatalogFileName == "" {
		return false, ""
	}
	if sp.CatalogSizeBytes == nil || *sp.CatalogSizeBytes <= 0 {
		return false, ""
	}
	return true, *sp.CatalogFileName
}
