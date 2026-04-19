package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/sender"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// Service wires together the conversation repo, channel registry, AI router,
// and peer modules (contact, sender profile) into a single facade for handlers.
type Service struct {
	repo        Repository
	channelRepo channel.Repository
	contacts    contact.Repository
	sender      sender.Repository
	registry    *channel.Registry
	ai          *ai.Router
	pool        *pgxpool.Pool // for reading businesses/lead_scores on-demand
}

func NewService(
	repo Repository,
	channelRepo channel.Repository,
	contacts contact.Repository,
	senderRepo sender.Repository,
	registry *channel.Registry,
	aiRouter *ai.Router,
	pool *pgxpool.Pool,
) *Service {
	return &Service{
		repo: repo, channelRepo: channelRepo, contacts: contacts,
		sender: senderRepo, registry: registry, ai: aiRouter, pool: pool,
	}
}

// StartFromContact creates a new conversation on the user's default Gmail channel
// for a given contact and optionally AI-drafts the opening message.
// Returns the conversation + the draft message (status=draft, not yet sent).
func (s *Service) StartFromContact(ctx context.Context, userID, contactID uuid.UUID, draftWithAI bool) (*StartResult, error) {
	c, err := s.contacts.Get(ctx, userID, contactID)
	if err != nil {
		return nil, fmt.Errorf("load contact: %w", err)
	}
	if c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		return nil, errors.New("contact has no primary email")
	}

	// Pick the default channel.
	chs, err := s.channelRepo.List(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	var chosen *domain.UserChannel
	for i, uc := range chs {
		if uc.Type == domain.ChannelTypeGmailOAuth && uc.Enabled {
			if uc.IsDefault || chosen == nil {
				chosen = &chs[i]
				if uc.IsDefault {
					break
				}
			}
		}
	}
	if chosen == nil {
		return nil, errors.New("no gmail channel connected — connect one under Settings → Channels")
	}

	// Create the conversation shell.
	conv, err := s.repo.Create(ctx, domain.Conversation{
		UserID:      userID,
		ContactID:   contactID,
		ChannelID:   chosen.ID,
		ChannelType: chosen.Type,
		Automation:  c.DefaultAutomation,
		Status:      domain.ConversationStatusActive,
	})
	if err != nil {
		return nil, err
	}

	result := &StartResult{Conversation: *conv}

	if draftWithAI {
		draft, err := s.draftInitial(ctx, userID, c)
		if err != nil {
			// Don't fail the whole request — the conversation exists, user can write manually.
			result.DraftError = err.Error()
			return result, nil
		}
		// Save as a pending-approval message.
		subject := draft.Subject
		body := draft.Body
		promptV := "outreach_draft_v1"
		m, err := s.repo.CreateMessage(ctx, domain.Message{
			ConversationID:  conv.ID,
			Direction:       domain.DirectionOutbound,
			ChannelType:     chosen.Type,
			Subject:         &subject,
			BodyText:        &body,
			AIGenerated:     true,
			AIPromptVersion: &promptV,
			Status:          domain.MessageStatusPendingApproval,
		})
		if err != nil {
			return result, err
		}
		result.DraftMessage = m
	}

	return result, nil
}

type StartResult struct {
	Conversation domain.Conversation `json:"conversation"`
	DraftMessage *domain.Message     `json:"draft_message,omitempty"`
	DraftError   string              `json:"draft_error,omitempty"`
}

// SendMessage sends a message through the conversation's channel.
// Either provide a draft message ID (we send the stored draft) OR provide body/subject directly.
func (s *Service) SendMessage(ctx context.Context, userID, conversationID uuid.UUID, req SendMessageRequest) (*domain.Message, error) {
	conv, err := s.repo.Get(ctx, userID, conversationID)
	if err != nil {
		return nil, fmt.Errorf("load conversation: %w", err)
	}

	// Resolve body — from stored draft or request body.
	var (
		subject string
		body    string
		msgID   uuid.UUID
		isDraft bool
	)
	if req.DraftMessageID != nil {
		msgs, err := s.repo.ListMessages(ctx, conv.ID)
		if err != nil {
			return nil, err
		}
		var draft *domain.Message
		for i := range msgs {
			if msgs[i].ID == *req.DraftMessageID {
				draft = &msgs[i]
				break
			}
		}
		if draft == nil {
			return nil, fmt.Errorf("draft %s not found in this conversation", *req.DraftMessageID)
		}
		if draft.Subject != nil {
			subject = *draft.Subject
		}
		if draft.BodyText != nil {
			body = *draft.BodyText
		}
		msgID = draft.ID
		isDraft = true
	} else {
		subject = req.Subject
		body = req.Body
	}
	if subject == "" || body == "" {
		return nil, errors.New("subject and body are required")
	}

	// Load channel and build Channel instance.
	uc, err := s.channelRepo.Get(ctx, userID, conv.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("load channel: %w", err)
	}
	ch, err := s.registry.Build(ctx, *uc)
	if err != nil {
		return nil, fmt.Errorf("build channel: %w", err)
	}

	// Look up recipient email from the contact.
	c, err := s.contacts.Get(ctx, userID, conv.ContactID)
	if err != nil {
		return nil, err
	}
	if c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		return nil, errors.New("contact has no email")
	}

	// Identify previous outbound external id for threading on replies.
	inReplyTo := ""
	if conv.ExternalThreadID != nil && *conv.ExternalThreadID != "" {
		msgs, _ := s.repo.ListMessages(ctx, conv.ID)
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].ExternalID != nil && *msgs[i].ExternalID != "" {
				inReplyTo = *msgs[i].ExternalID
				break
			}
		}
	}

	// Send through the channel.
	sendReq := channel.SendRequest{
		To:                  *c.PrimaryEmail,
		Subject:             subject,
		BodyText:            body,
		InReplyToExternalID: inReplyTo,
	}
	if conv.ExternalThreadID != nil {
		sendReq.ThreadID = *conv.ExternalThreadID
	}
	res, err := ch.Send(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("channel send: %w", err)
	}

	// Persist the message (either replacing the draft status, or fresh).
	now := time.Now().UTC()
	if isDraft {
		// Update the draft row to sent.
		_, err = s.pool.Exec(ctx,
			`UPDATE messages SET
			   external_id = $1, status = $2, sent_at = $3,
			   in_reply_to_external_id = NULLIF($4, '')
			 WHERE id = $5`,
			res.ExternalMessageID, domain.MessageStatusSent, now, inReplyTo, msgID,
		)
		if err != nil {
			return nil, fmt.Errorf("update sent draft: %w", err)
		}
	} else {
		var irt *string
		if inReplyTo != "" {
			irt = &inReplyTo
		}
		m, err := s.repo.CreateMessage(ctx, domain.Message{
			ConversationID:      conv.ID,
			Direction:           domain.DirectionOutbound,
			ChannelType:         conv.ChannelType,
			ExternalID:          &res.ExternalMessageID,
			InReplyToExternalID: irt,
			Subject:             &subject,
			BodyText:            &body,
			Status:              domain.MessageStatusSent,
			SentAt:              &now,
		})
		if err != nil {
			return nil, err
		}
		msgID = m.ID
	}

	// Update conversation bookkeeping (first send → save thread id).
	if conv.ExternalThreadID == nil || *conv.ExternalThreadID == "" {
		_, err = s.pool.Exec(ctx,
			`UPDATE conversations SET external_thread_id = $1, subject = COALESCE(subject, $2), updated_at = now() WHERE id = $3`,
			res.ExternalThreadID, subject, conv.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("persist thread id: %w", err)
		}
	}
	_ = s.repo.UpdateLast(ctx, conv.ID, domain.DirectionOutbound, now, false)

	// Mark contact as contacted if still in 'lead' stage.
	if c.PipelineStage == domain.PipelineLead {
		c.PipelineStage = domain.PipelineContacted
		_, _ = s.contacts.Update(ctx, *c)
	}

	// Return the persisted message (refetch by id for up-to-date status).
	msgs, _ := s.repo.ListMessages(ctx, conv.ID)
	for i := range msgs {
		if msgs[i].ID == msgID {
			return &msgs[i], nil
		}
	}
	return nil, nil
}

type SendMessageRequest struct {
	DraftMessageID *uuid.UUID `json:"draft_message_id,omitempty"`
	Subject        string     `json:"subject,omitempty"`
	Body           string     `json:"body,omitempty"`
}

// DraftReply asks the AI to suggest a reply to the latest inbound message.
// Saves a pending_approval message and returns it.
func (s *Service) DraftReply(ctx context.Context, userID, conversationID uuid.UUID) (*domain.Message, error) {
	conv, err := s.repo.Get(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	msgs, err := s.repo.ListMessages(ctx, conv.ID)
	if err != nil {
		return nil, err
	}
	c, err := s.contacts.Get(ctx, userID, conv.ContactID)
	if err != nil {
		return nil, err
	}

	sp, _ := s.sender.Get(ctx, userID)
	if sp == nil {
		sp = &domain.SenderProfile{UserID: userID, Tone: "formal"}
	}
	business := s.loadBusinessJSON(ctx, c.BusinessID)

	// Render transcript.
	var tb strings.Builder
	for _, m := range msgs {
		who := "YOU"
		if m.Direction == domain.DirectionInbound {
			who = "THEM"
		}
		subj := ""
		if m.Subject != nil {
			subj = *m.Subject
		}
		body := ""
		if m.BodyText != nil {
			body = *m.BodyText
		} else if m.BodyHTML != nil {
			body = *m.BodyHTML
		}
		tb.WriteString(fmt.Sprintf("--- %s (%s) ---\nSubject: %s\n%s\n\n", who, m.Direction, subj, body))
	}

	prompt := prompts.BuildOutreachReplyPrompt(prompts.OutreachReplyInput{
		SenderProfileJSON: prompts.EncodeJSON(sp),
		ContactJSON:       prompts.EncodeJSON(c),
		BusinessJSON:      business,
		ConversationText:  tb.String(),
	})
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := s.ai.CompleteJSON(ctxAI, "outreach_reply", prompt.Prompt, prompt.System, 30*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("ai reply: %w", err)
	}
	var out prompts.OutreachReplyResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse ai reply: %w", err)
	}
	if out.Body == "" {
		return nil, errors.New("ai returned empty body")
	}

	// Subject — reuse conversation subject with Re: prefix if not already.
	subject := ""
	if conv.Subject != nil {
		subject = *conv.Subject
	}
	if !strings.HasPrefix(strings.ToLower(subject), "re:") && subject != "" {
		subject = "Re: " + subject
	}
	promptV := "outreach_reply_v1"
	return s.repo.CreateMessage(ctx, domain.Message{
		ConversationID:  conv.ID,
		Direction:       domain.DirectionOutbound,
		ChannelType:     conv.ChannelType,
		Subject:         &subject,
		BodyText:        &out.Body,
		AIGenerated:     true,
		AIPromptVersion: &promptV,
		Status:          domain.MessageStatusPendingApproval,
	})
}

// --- internals ---------------------------------------------------------

func (s *Service) draftInitial(ctx context.Context, userID uuid.UUID, c *domain.Contact) (*prompts.OutreachDraftResult, error) {
	sp, _ := s.sender.Get(ctx, userID)
	if sp == nil {
		sp = &domain.SenderProfile{UserID: userID, Tone: "formal"}
	}
	businessJSON := s.loadBusinessJSON(ctx, c.BusinessID)
	shipmentContext, _ := s.loadShipmentContext(ctx, c.BusinessID)

	prompt := prompts.BuildOutreachDraftPrompt(prompts.OutreachDraftInput{
		SenderProfileJSON:   prompts.EncodeJSON(sp),
		CampaignPositioning: "",
		ContactJSON:         prompts.EncodeJSON(c),
		BusinessJSON:        businessJSON,
		ShipmentContext:     shipmentContext,
		LeadScoreJSON:       "",
	})
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := s.ai.CompleteJSON(ctxAI, "outreach_draft", prompt.Prompt, prompt.System, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	var out prompts.OutreachDraftResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Subject == "" || out.Body == "" {
		return nil, errors.New("ai returned empty subject/body")
	}
	return &out, nil
}

func (s *Service) loadBusinessJSON(ctx context.Context, id uuid.UUID) string {
	var b domain.Business
	row := s.pool.QueryRow(ctx,
		`SELECT id, name, website, country_code, city, address, phone, email,
		        industry, sub_industry, business_type, description, shipment_data
		 FROM businesses WHERE id = $1`, id,
	)
	if err := row.Scan(&b.ID, &b.Name, &b.Website, &b.CountryCode, &b.City,
		&b.Address, &b.Phone, &b.Email, &b.Industry, &b.SubIndustry,
		&b.BusinessType, &b.Description, &b.ShipmentData); err != nil {
		return fmt.Sprintf("(business %s not loadable: %v)", id, err)
	}
	return prompts.EncodeJSON(b)
}

func (s *Service) loadShipmentContext(ctx context.Context, id uuid.UUID) (string, error) {
	var rawJSON []byte
	err := s.pool.QueryRow(ctx, `SELECT shipment_data FROM businesses WHERE id = $1`, id).Scan(&rawJSON)
	if err != nil || len(rawJSON) == 0 {
		return "", nil
	}
	// Render as human-readable summary (re-using the shape from scoring.go).
	var sd struct {
		TransactionCount int            `json:"transaction_count"`
		TotalWeightKG    float64        `json:"total_weight_kg"`
		TotalValueUSD    float64        `json:"total_value_usd"`
		LastShipmentDate string         `json:"last_shipment_date"`
		Products         []string       `json:"products"`
		Suppliers        []struct {
			Name    string `json:"name"`
			Country string `json:"country"`
		} `json:"suppliers"`
		TrustTier string `json:"trust_tier"`
	}
	if err := json.Unmarshal(rawJSON, &sd); err != nil {
		return "", err
	}
	var parts []string
	if sd.TransactionCount > 0 {
		parts = append(parts, fmt.Sprintf("%d shipments totaling %.0f kg", sd.TransactionCount, sd.TotalWeightKG))
	}
	if sd.TotalValueUSD > 0 {
		parts = append(parts, fmt.Sprintf("$%.0f declared value", sd.TotalValueUSD))
	}
	if sd.LastShipmentDate != "" {
		parts = append(parts, "last shipment "+sd.LastShipmentDate)
	}
	if len(sd.Products) > 0 {
		parts = append(parts, "products: "+strings.Join(sd.Products, ", "))
	}
	if len(sd.Suppliers) > 0 {
		var sups []string
		for _, s := range sd.Suppliers {
			sups = append(sups, s.Name+" ("+s.Country+")")
		}
		parts = append(parts, "current suppliers: "+strings.Join(sups, "; "))
	}
	if sd.TrustTier != "" {
		parts = append(parts, "trust tier: "+sd.TrustTier)
	}
	return strings.Join(parts, " | "), nil
}
