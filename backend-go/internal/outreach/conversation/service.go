package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/replyguidance"
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

// StartFromContact returns an existing active conversation for the contact if
// one exists (click-twice idempotency), otherwise creates a new one and
// optionally AI-drafts the opening message.
func (s *Service) StartFromContact(ctx context.Context, userID, contactID uuid.UUID, draftWithAI bool) (*StartResult, error) {
	c, err := s.contacts.Get(ctx, userID, contactID)
	if err != nil {
		return nil, fmt.Errorf("load contact: %w", err)
	}
	if c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		return nil, errors.New("contact has no primary email")
	}

	// Reuse existing active conversation if one exists.
	if existing, err := s.repo.FindActiveByContact(ctx, userID, contactID); err == nil && existing != nil {
		return &StartResult{Conversation: *existing, AlreadyExisted: true}, nil
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
			result.DraftError = err.Error()
			return result, nil
		}
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
	Conversation   domain.Conversation `json:"conversation"`
	DraftMessage   *domain.Message     `json:"draft_message,omitempty"`
	DraftError     string              `json:"draft_error,omitempty"`
	AlreadyExisted bool                `json:"already_existed,omitempty"`
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
		// Only an actual draft may be "sent as draft" — a stale tab passing an
		// already-sent message id must not re-send it.
		if draft.Status != domain.MessageStatusPendingApproval && draft.Status != domain.MessageStatusDraft {
			return nil, fmt.Errorf("message %s is not a pending draft (status %s)", draft.ID, draft.Status)
		}
		// The user's edited text wins: the compose box promises "your edits will
		// be sent". The stored draft is only the fallback when the request omits
		// a field, and we persist what was actually sent back onto the row.
		if draft.Subject != nil {
			subject = *draft.Subject
		}
		if draft.BodyText != nil {
			body = *draft.BodyText
		}
		if strings.TrimSpace(req.Subject) != "" {
			subject = req.Subject
		}
		if strings.TrimSpace(req.Body) != "" {
			body = req.Body
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
	// Approving an AI draft must re-check suppression — the draft may have been
	// created before the contact unsubscribed (keyword/one-click can race the
	// reply branch). Hand-typed manual messages stay allowed: confirming an
	// opt-out by hand is legitimate.
	if isDraft && compliance.IsSuppressed(c) {
		return nil, errors.New("contact has unsubscribed — automated drafts can no longer be sent (you can still write a manual message)")
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
	// Optionally attach the brand's catalog.
	if req.AttachCatalog {
		brand := s.brandForConv(ctx, conv, userID)
		fname, mimeType, data, err := s.sender.GetCatalogData(ctx, brand.ID)
		if err == nil && len(data) > 0 {
			sendReq.Attachments = []channel.Attachment{{
				Filename: fname,
				MimeType: mimeType,
				Data:     data,
			}}
		}
		// If no catalog is uploaded we silently skip — the flag is a hint.
	}
	res, err := ch.Send(ctx, sendReq)
	if err != nil {
		return nil, fmt.Errorf("channel send: %w", err)
	}

	// Persist the message (either replacing the draft status, or fresh).
	now := time.Now().UTC()
	if isDraft {
		// Update the draft row to sent, persisting the text that actually went
		// out (the user may have edited the draft in the compose box). body_html
		// is cleared — it could only be stale relative to the edited text.
		_, err = s.pool.Exec(ctx,
			`UPDATE messages SET
			   external_id = $1, status = $2, sent_at = $3,
			   in_reply_to_external_id = NULLIF($4, ''),
			   subject = $5, body_text = $6, body_html = NULL
			 WHERE id = $7`,
			res.ExternalMessageID, domain.MessageStatusSent, now, inReplyTo, subject, body, msgID,
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
	// When true, fetch the user's sender-profile catalog and attach it.
	// No-op if no catalog is uploaded.
	AttachCatalog bool `json:"attach_catalog,omitempty"`
}

// DraftReply is now state-aware — it inspects the conversation and picks the
// right prompt:
//   - no sent/received messages yet → outreach_draft (cold initial)
//   - last real message is inbound → outreach_reply (mode=reply)
//   - last real message is outbound → outreach_reply (mode=followup)
//
// Before generating, it wipes any stale pending_approval draft so we don't
// accumulate half-written AI messages.
func (s *Service) DraftReply(ctx context.Context, userID, conversationID uuid.UUID) (*domain.Message, error) {
	conv, err := s.repo.Get(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.DeletePendingDrafts(ctx, conv.ID); err != nil {
		return nil, fmt.Errorf("cleanup old drafts: %w", err)
	}
	msgs, err := s.repo.ListMessages(ctx, conv.ID)
	if err != nil {
		return nil, err
	}
	c, err := s.contacts.Get(ctx, userID, conv.ContactID)
	if err != nil {
		return nil, err
	}

	sp := s.brandForConv(ctx, conv, userID)
	businessJSON := s.loadBusinessJSON(ctx, c.BusinessID)
	shipmentCtx, _ := s.loadShipmentContext(ctx, c.BusinessID)
	leadScoreJSON := s.loadLeadScoreJSON(ctx, userID, c.BusinessID)

	// Only count real sent/received messages (skip drafts).
	sentMsgs := make([]domain.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Status != domain.MessageStatusPendingApproval && m.Status != domain.MessageStatusDraft {
			sentMsgs = append(sentMsgs, m)
		}
	}

	// State: initial (no real messages), reply (last is inbound), followup (last is outbound).
	state := "initial"
	if len(sentMsgs) > 0 {
		last := sentMsgs[len(sentMsgs)-1]
		if last.Direction == domain.DirectionInbound {
			state = "reply"
		} else {
			state = "followup"
		}
	}

	switch state {
	case "initial":
		return s.draftInitialMessage(ctx, userID, conv, c, sp, businessJSON, shipmentCtx, leadScoreJSON)
	default:
		return s.draftReplyOrFollowup(ctx, userID, conv, c, sp, businessJSON, leadScoreJSON, sentMsgs, state)
	}
}

// draftInitialMessage is the "first cold email" path — used when the user
// clicks AI draft on an empty conversation.
func (s *Service) draftInitialMessage(
	ctx context.Context,
	userID uuid.UUID,
	conv *domain.Conversation,
	c *domain.Contact,
	sp *domain.SenderProfile,
	businessJSON, shipmentCtx, leadScoreJSON string,
) (*domain.Message, error) {
	hasCatalog, catalogName := senderCatalog(sp)
	priorBlock := s.loadPriorConversationsBlock(ctx, userID, c.ID, conv.ID)

	in := prompts.OutreachDraftInput{
		SenderProfileJSON:         prompts.EncodeJSON(sp),
		ContactJSON:               prompts.EncodeJSON(c),
		BusinessJSON:              businessJSON,
		ShipmentContext:           shipmentCtx,
		LeadScoreJSON:             leadScoreJSON,
		PriorConversationsContext: priorBlock,
		HasCatalog:                hasCatalog,
		CatalogFilename:           catalogName,
	}
	out, err := s.draftOutreachWithGuard(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	out.Body = prompts.WithSignature(out.Body, sp.Signature)
	promptV := "outreach_draft_v1"
	return s.repo.CreateMessage(ctx, domain.Message{
		ConversationID:  conv.ID,
		Direction:       domain.DirectionOutbound,
		ChannelType:     conv.ChannelType,
		Subject:         &out.Subject,
		BodyText:        &out.Body,
		AIGenerated:     true,
		AIPromptVersion: &promptV,
		Status:          domain.MessageStatusPendingApproval,
	})
}

// loadPriorConversationsBlock pulls up to 5 prior threads with this contact
// across every campaign/channel and renders them into the compact text the
// AI prompt expects. The "current" conversation is excluded so a freshly
// created empty thread doesn't show up. Returns "" on any error or when
// nothing relevant exists.
func (s *Service) loadPriorConversationsBlock(ctx context.Context, userID, contactID, excludeConvID uuid.UUID) string {
	threads, err := s.repo.ListByContact(ctx, userID, contactID, 5, 10)
	if err != nil || len(threads) == 0 {
		return ""
	}
	filtered := make([]ThreadWithMessages, 0, len(threads))
	for _, t := range threads {
		if t.Conversation.ID == excludeConvID {
			continue
		}
		filtered = append(filtered, t)
	}
	if len(filtered) == 0 {
		return ""
	}
	return RenderPriorConversations(filtered, time.Now())
}

// draftOutreachWithGuard runs one AI completion, validates the body with the
// shared post-gen guard, and re-runs once with StricterRetry=true on any
// violation. Falls back to the first attempt on retry failure so a partial
// success still produces a draft.
func (s *Service) draftOutreachWithGuard(ctx context.Context, userID uuid.UUID, in prompts.OutreachDraftInput) (*prompts.OutreachDraftResult, error) {
	first, err := s.draftOutreachOnce(ctx, userID, in)
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
	slog.Warn("outreach drafter: guard violations, retrying",
		"violations", violations, "subject", first.Subject,
	)
	in.StricterRetry = true
	second, err := s.draftOutreachOnce(ctx, userID, in)
	if err != nil {
		return first, nil
	}
	return second, nil
}

func (s *Service) draftOutreachOnce(ctx context.Context, userID uuid.UUID, in prompts.OutreachDraftInput) (*prompts.OutreachDraftResult, error) {
	prompt := prompts.BuildOutreachDraftPrompt(in)
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := s.ai.CompleteJSON(ctxAI, "outreach_draft", prompt.Prompt, prompt.System, 0)
	if err != nil {
		return nil, fmt.Errorf("ai draft: %w", err)
	}
	var out prompts.OutreachDraftResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("parse ai draft: %w", err)
	}
	if out.Body == "" || out.Subject == "" {
		return nil, errors.New("ai returned empty subject/body")
	}
	return &out, nil
}

// draftReplyOrFollowup handles both the reply and followup states using the
// same prompt with different modes.
func (s *Service) draftReplyOrFollowup(
	ctx context.Context,
	userID uuid.UUID,
	conv *domain.Conversation,
	c *domain.Contact,
	sp *domain.SenderProfile,
	businessJSON, leadScoreJSON string,
	sentMsgs []domain.Message,
	state string,
) (*domain.Message, error) {
	var tb strings.Builder
	for _, m := range sentMsgs {
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

	hasCatalog, catalogName := senderCatalog(sp)
	guidance := ""
	if state == "reply" && sp != nil {
		guidance = replyguidance.Load(ctx, s.pool, conv.ID, conv.CampaignID, sp.ID)
	}
	in := prompts.OutreachReplyInput{
		Mode:              state,
		SenderProfileJSON: prompts.EncodeJSON(sp),
		ContactJSON:       prompts.EncodeJSON(c),
		BusinessJSON:      businessJSON,
		LeadScoreJSON:     leadScoreJSON,
		ConversationText:  tb.String(),
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
		Guidance:          guidance,
	}
	out, err := s.replyOutreachWithGuard(ctx, userID, in)
	if err != nil {
		return nil, err
	}

	subject := ""
	if conv.Subject != nil {
		subject = *conv.Subject
	}
	if state == "reply" && subject != "" && !strings.HasPrefix(strings.ToLower(subject), "re:") {
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

// RefineDraft rewrites the conversation's current pending draft to incorporate
// the user's instruction, re-running the reply prompt + guard, and updates the
// draft body in place. If remember is true, the instruction is saved as a
// per-brand lesson for this kind of reply (matched later by tags + sentiment).
// RefineDraft rewrites a pending draft per the user's instruction. The prompt
// family and guard cap follow the draft's actual state — a cold opener refines
// through the cold-draft prompt (and may rewrite the subject), a reply/followup
// through the reply prompt — so refinement never mangles a compliant draft with
// the wrong word cap. previousBody, when non-empty, is the user's current
// compose-box text (their manual edits), preferred over the stored row.
func (s *Service) RefineDraft(ctx context.Context, userID, conversationID, draftID uuid.UUID, instruction, previousBody string, remember bool) (*domain.Message, error) {
	conv, err := s.repo.Get(ctx, userID, conversationID)
	if err != nil {
		return nil, err
	}
	draft, err := s.repo.GetMessage(ctx, draftID)
	if err != nil {
		return nil, err
	}
	if draft.ConversationID != conv.ID ||
		(draft.Status != domain.MessageStatusPendingApproval && draft.Status != domain.MessageStatusDraft) {
		return nil, domain.ErrNotFound
	}
	prev := ""
	if draft.BodyText != nil {
		prev = *draft.BodyText
	}
	if strings.TrimSpace(previousBody) != "" {
		prev = previousBody // the user's in-progress edits win
	}

	msgs, err := s.repo.ListMessages(ctx, conv.ID)
	if err != nil {
		return nil, err
	}
	c, err := s.contacts.Get(ctx, userID, conv.ContactID)
	if err != nil {
		return nil, err
	}
	sp := s.brandForConv(ctx, conv, userID)
	businessJSON := s.loadBusinessJSON(ctx, c.BusinessID)
	leadScoreJSON := s.loadLeadScoreJSON(ctx, userID, c.BusinessID)

	// Determine the draft's state from the real (sent/received) history —
	// mirrors DraftReply's dispatch.
	var tb strings.Builder
	state := "initial"
	for _, m := range msgs {
		if m.Status == domain.MessageStatusPendingApproval || m.Status == domain.MessageStatusDraft {
			continue
		}
		if m.Direction == domain.DirectionInbound {
			state = "reply"
		} else {
			state = "followup"
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
		who := "YOU"
		if m.Direction == domain.DirectionInbound {
			who = "THEM"
		}
		tb.WriteString(fmt.Sprintf("--- %s (%s) ---\nSubject: %s\n%s\n\n", who, m.Direction, subj, body))
	}

	hasCatalog, catalogName := senderCatalog(sp)

	if state == "initial" {
		// Refining a cold opener: use the cold-draft prompt + cold guard cap,
		// and let the model revise the subject too.
		shipmentCtx, _ := s.loadShipmentContext(ctx, c.BusinessID)
		din := prompts.OutreachDraftInput{
			SenderProfileJSON: prompts.EncodeJSON(sp),
			ContactJSON:       prompts.EncodeJSON(c),
			BusinessJSON:      businessJSON,
			ShipmentContext:   shipmentCtx,
			LeadScoreJSON:     leadScoreJSON,
			HasCatalog:        hasCatalog,
			CatalogFilename:   catalogName,
			RefineInstruction: instruction,
			PreviousDraft:     prev,
		}
		out, err := s.draftOutreachWithGuard(ctx, userID, din)
		if err != nil {
			return nil, err
		}
		out.Body = prompts.WithSignature(out.Body, sp.Signature)
		if err := s.repo.UpdateMessageDraft(ctx, draft.ID, out.Subject, out.Body); err != nil {
			return nil, err
		}
		// No lesson capture for cold drafts: lessons are reply guidance, keyed
		// to a buyer answer's sentiment/tags — a cold opener has neither.
		return s.repo.GetMessage(ctx, draft.ID)
	}

	guidance := ""
	if state == "reply" && sp != nil {
		guidance = replyguidance.Load(ctx, s.pool, conv.ID, conv.CampaignID, sp.ID)
	}
	in := prompts.OutreachReplyInput{
		Mode:              state, // "reply" or "followup" — picks prompt wording AND guard cap
		SenderProfileJSON: prompts.EncodeJSON(sp),
		ContactJSON:       prompts.EncodeJSON(c),
		BusinessJSON:      businessJSON,
		LeadScoreJSON:     leadScoreJSON,
		ConversationText:  tb.String(),
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
		Guidance:          guidance,
		RefineInstruction: instruction,
		PreviousDraft:     prev,
	}
	out, err := s.replyOutreachWithGuard(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateMessageBody(ctx, draft.ID, out.Body); err != nil {
		return nil, err
	}
	// Lessons only make sense for replies — remembering from a followup/cold
	// refine would store an untagged neutral lesson that contaminates every
	// future neutral reply for the brand.
	if remember && state == "reply" && strings.TrimSpace(instruction) != "" && sp != nil {
		if err := replyguidance.RememberLesson(ctx, s.pool, userID, sp.ID, conv.ID, instruction); err != nil {
			slog.Warn("refine: remember lesson failed", "conversation_id", conv.ID, "error", err)
		}
	}
	return s.repo.GetMessage(ctx, draft.ID)
}

// replyOutreachWithGuard runs the reply/followup AI call, validates the
// body with the post-gen guard (mode picked from in.Mode), and retries once
// with StricterRetry=true on violation. Falls back to first attempt on
// retry failure.
func (s *Service) replyOutreachWithGuard(ctx context.Context, userID uuid.UUID, in prompts.OutreachReplyInput) (*prompts.OutreachReplyResult, error) {
	first, err := s.replyOutreachOnce(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	mode := prompts.DraftModeReply
	if in.Mode == "followup" {
		mode = prompts.DraftModeFollowup
	}
	violations := prompts.ValidateOutreachBody(first.Body, in.HasCatalog, mode)
	if len(violations) == 0 {
		return first, nil
	}
	slog.Warn("outreach replier: guard violations, retrying",
		"violations", violations, "mode", in.Mode,
	)
	in.StricterRetry = true
	second, err := s.replyOutreachOnce(ctx, userID, in)
	if err != nil {
		return first, nil
	}
	return second, nil
}

func (s *Service) replyOutreachOnce(ctx context.Context, userID uuid.UUID, in prompts.OutreachReplyInput) (*prompts.OutreachReplyResult, error) {
	prompt := prompts.BuildOutreachReplyPrompt(in)
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := s.ai.CompleteJSON(ctxAI, "outreach_reply", prompt.Prompt, prompt.System, 0)
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
	return &out, nil
}

// loadLeadScoreJSON returns the best (highest overall_score, newest) lead
// score for a business scoped to a user. Returns "" if no score found.
func (s *Service) loadLeadScoreJSON(ctx context.Context, userID, businessID uuid.UUID) string {
	var (
		overall       *int
		purchase      *int
		dealSize      *int
		urgency       *int
		fit           *int
		access        *int
		rationale     *string
		recApproach   *string
	)
	err := s.pool.QueryRow(ctx,
		`SELECT overall_score, purchase_likelihood, deal_size_potential,
		        urgency_score, fit_score, accessibility_score,
		        scoring_rationale, recommended_approach
		 FROM lead_scores
		 WHERE business_id = $1 AND user_id = $2
		 ORDER BY overall_score DESC NULLS LAST, scored_at DESC
		 LIMIT 1`, businessID, userID,
	).Scan(&overall, &purchase, &dealSize, &urgency, &fit, &access, &rationale, &recApproach)
	if err != nil {
		return ""
	}
	return prompts.EncodeJSON(map[string]any{
		"overall_score":        overall,
		"purchase_likelihood":  purchase,
		"deal_size_potential":  dealSize,
		"urgency_score":        urgency,
		"fit_score":            fit,
		"accessibility_score":  access,
		"scoring_rationale":    rationale,
		"recommended_approach": recApproach,
	})
}

// --- internals ---------------------------------------------------------

func (s *Service) draftInitial(ctx context.Context, userID uuid.UUID, c *domain.Contact) (*prompts.OutreachDraftResult, error) {
	sp := s.defaultBrand(ctx, userID)
	businessJSON := s.loadBusinessJSON(ctx, c.BusinessID)
	shipmentContext, _ := s.loadShipmentContext(ctx, c.BusinessID)
	leadScoreJSON := s.loadLeadScoreJSON(ctx, userID, c.BusinessID)
	priorBlock := s.loadPriorConversationsBlock(ctx, userID, c.ID, uuid.Nil)

	hasCatalog, catalogName := senderCatalog(sp)
	in := prompts.OutreachDraftInput{
		SenderProfileJSON:         prompts.EncodeJSON(sp),
		CampaignPositioning:       "",
		ContactJSON:               prompts.EncodeJSON(c),
		BusinessJSON:              businessJSON,
		ShipmentContext:           shipmentContext,
		LeadScoreJSON:             leadScoreJSON,
		PriorConversationsContext: priorBlock,
		HasCatalog:                hasCatalog,
		CatalogFilename:           catalogName,
	}
	out, err := s.draftOutreachWithGuard(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	out.Body = prompts.WithSignature(out.Body, sp.Signature)
	return out, nil
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

// defaultBrand resolves the user's Default brand. Returns a minimal stub
// (never nil) so callers always have a usable profile.
func (s *Service) defaultBrand(ctx context.Context, userID uuid.UUID) *domain.SenderProfile {
	sp, err := s.sender.DefaultForUser(ctx, userID)
	if err != nil || sp == nil {
		return &domain.SenderProfile{UserID: userID, Tone: "formal"}
	}
	return sp
}

// brandForConv resolves the brand a conversation should send/draft as. If the
// conversation belongs to a campaign (Email Group) with an assigned brand, use
// it; otherwise the user's Default brand. Avoids importing the campaign
// package (cycle) by reading the column directly.
func (s *Service) brandForConv(ctx context.Context, conv *domain.Conversation, userID uuid.UUID) *domain.SenderProfile {
	if conv != nil && conv.CampaignID != nil {
		var profileID *uuid.UUID
		if err := s.pool.QueryRow(ctx,
			`SELECT sender_profile_id FROM campaigns WHERE id = $1`, *conv.CampaignID,
		).Scan(&profileID); err == nil && profileID != nil {
			if sp, err := s.sender.GetByID(ctx, userID, *profileID); err == nil && sp != nil {
				return sp
			}
		}
	}
	return s.defaultBrand(ctx, userID)
}

// senderCatalog returns whether the sender profile has a catalog uploaded,
// and the file name to surface to the AI draft.
func senderCatalog(sp *domain.SenderProfile) (bool, string) {
	if sp == nil || sp.CatalogFileName == nil || *sp.CatalogFileName == "" {
		return false, ""
	}
	if sp.CatalogSizeBytes == nil || *sp.CatalogSizeBytes <= 0 {
		return false, ""
	}
	return true, *sp.CatalogFileName
}
