package sequence

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
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/internalsched"
	"github.com/batuhan/marketintel/internal/outreach/leadctx"
	"github.com/batuhan/marketintel/internal/outreach/sender"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// Engine processes due sequence_runs each scheduler tick. For each run it
// loads the conversation, evaluates the step's trigger against conversation
// state, and either fires the action (send message / mark cold / advance
// stage / notify) or pushes next_run_at out by another minute when the
// trigger condition isn't yet met.
type Engine struct {
	repo         Repository
	conv         conversation.Repository
	contacts     contact.Repository
	channels     channel.Repository
	sender       sender.Repository
	registry     *channel.Registry
	ai           *ai.Router
	leadCtx      *leadctx.Loader
	pool         *pgxpool.Pool
	publicAPIURL string
	maxPerTick   int
}

func NewEngine(
	repo Repository,
	convRepo conversation.Repository,
	contacts contact.Repository,
	channels channel.Repository,
	senderRepo sender.Repository,
	registry *channel.Registry,
	aiRouter *ai.Router,
	leadCtxLoader *leadctx.Loader,
	pool *pgxpool.Pool,
	publicAPIURL string,
) *Engine {
	return &Engine{
		repo: repo, conv: convRepo, contacts: contacts, channels: channels,
		sender: senderRepo, registry: registry, ai: aiRouter,
		leadCtx: leadCtxLoader, pool: pool, publicAPIURL: publicAPIURL,
		maxPerTick: 100,
	}
}

func (e *Engine) Tick(ctx context.Context) (internalsched.TickResult, error) {
	res := internalsched.TickResult{Component: "sequence_engine"}
	now := time.Now().UTC()
	runs, err := e.repo.DueRuns(ctx, now, e.maxPerTick)
	if err != nil {
		return res, err
	}
	for _, run := range runs {
		if err := e.processOne(ctx, run); err != nil {
			res.Errors++
			slog.Warn("sequence engine: run failed", "run_id", run.ID, "error", err)
			continue
		}
		res.Processed++
	}
	return res, nil
}

func (e *Engine) processOne(ctx context.Context, run domain.SequenceRun) error {
	step, err := e.repo.GetStep(ctx, run.SequenceID, run.CurrentStep)
	if err != nil {
		// No more steps → completed.
		if errors.Is(err, domain.ErrNotFound) {
			return e.repo.CompleteRun(ctx, run.ID, domain.SequenceRunCompleted)
		}
		return err
	}

	conv, err := e.loadConversation(ctx, run.ConversationID)
	if err != nil {
		return fmt.Errorf("load conversation: %w", err)
	}
	if conv.Status != domain.ConversationStatusActive {
		// Conversation is paused or closed — pause the run.
		return e.repo.CompleteRun(ctx, run.ID, domain.SequenceRunPaused)
	}

	// Evaluate the trigger. If it doesn't match yet, push next_run_at out by
	// a minute and try again next tick (don't advance the step).
	match, err := e.evaluateTrigger(ctx, step, conv)
	if err != nil {
		return fmt.Errorf("trigger eval: %w", err)
	}
	if !match {
		return e.repo.BumpRun(ctx, run.ID, time.Now().Add(60*time.Second))
	}

	// Fire the action.
	if err := e.fireAction(ctx, step, conv); err != nil {
		return fmt.Errorf("fire action: %w", err)
	}

	// Schedule the next step (if any), else complete.
	next := run.CurrentStep + 1
	nextStep, err := e.repo.GetStep(ctx, run.SequenceID, next)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return e.repo.CompleteRun(ctx, run.ID, domain.SequenceRunCompleted)
		}
		return err
	}
	when := time.Now().UTC().Add(time.Duration(nextStep.WaitDays) * 24 * time.Hour)
	return e.repo.AdvanceRun(ctx, run.ID, next, when)
}

// evaluateTrigger checks whether the step's trigger condition is met against
// the current conversation state. Returns (match, err).
func (e *Engine) evaluateTrigger(ctx context.Context, step *domain.SequenceStep, conv *domain.Conversation) (bool, error) {
	switch step.Trigger {
	case domain.SequenceTriggerAlways:
		return true, nil
	case domain.SequenceTriggerNoReply:
		// Trigger fires only if the last message was outbound (no reply yet)
		// AND wait_days has actually elapsed since that message.
		if conv.LastDirection == nil || *conv.LastDirection != domain.DirectionOutbound {
			return false, nil
		}
		if conv.LastMessageAt == nil {
			return false, nil
		}
		return time.Since(*conv.LastMessageAt) >= time.Duration(step.WaitDays)*24*time.Hour, nil
	case domain.SequenceTriggerAnyReply:
		return conv.LastDirection != nil && *conv.LastDirection == domain.DirectionInbound, nil
	case domain.SequenceTriggerPositiveReply:
		// Cheap path: must already have an inbound. Then run sentiment AI on
		// the most recent inbound body.
		if conv.LastDirection == nil || *conv.LastDirection != domain.DirectionInbound {
			return false, nil
		}
		msgs, err := e.conv.ListMessages(ctx, conv.ID)
		if err != nil {
			return false, err
		}
		var lastInbound *domain.Message
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Direction == domain.DirectionInbound {
				lastInbound = &msgs[i]
				break
			}
		}
		if lastInbound == nil {
			return false, nil
		}
		body := ""
		if lastInbound.BodyText != nil {
			body = *lastInbound.BodyText
		}
		return e.classifySentimentPositive(ctx, conv.UserID, lastInbound, body)
	default:
		// Unknown trigger — never fire.
		return false, nil
	}
}

func (e *Engine) classifySentimentPositive(ctx context.Context, userID uuid.UUID, m *domain.Message, body string) (bool, error) {
	prompt := prompts.BuildOutreachSentimentPrompt(prompts.OutreachSentimentInput{
		Subject: derefSubject(m),
		Body:    body,
	})
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := e.ai.CompleteJSON(ctxAI, "outreach_sentiment", prompt.Prompt, prompt.System, 24*time.Hour)
	if err != nil {
		return false, err
	}
	var out prompts.OutreachSentimentResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return false, err
	}
	return out.Sentiment == "positive" && out.Confidence >= 0.5, nil
}

// fireAction executes the step's action against the conversation.
func (e *Engine) fireAction(ctx context.Context, step *domain.SequenceStep, conv *domain.Conversation) error {
	switch step.Action {
	case domain.SequenceActionSendMessage:
		return e.actionSendMessage(ctx, step, conv)
	case domain.SequenceActionMarkCold:
		return e.contacts.MarkUnsubscribed(ctx, conv.ContactID, "sequence_marked_cold")
	case domain.SequenceActionAdvanceStage:
		c, err := e.contacts.Get(ctx, conv.UserID, conv.ContactID)
		if err != nil || c == nil {
			return err
		}
		stage := domain.PipelineQualified
		if step.PromptOverride != nil && *step.PromptOverride != "" {
			stage = *step.PromptOverride
		}
		c.PipelineStage = stage
		_, err = e.contacts.Update(ctx, *c)
		return err
	case domain.SequenceActionNotifyUser:
		// V1: just log. UI surface for notifications is a future story.
		slog.Info("sequence notify_user", "conversation_id", conv.ID, "step", step.StepNumber)
		return nil
	default:
		return fmt.Errorf("unknown action %s", step.Action)
	}
}

func (e *Engine) actionSendMessage(ctx context.Context, step *domain.SequenceStep, conv *domain.Conversation) error {
	c, err := e.contacts.Get(ctx, conv.UserID, conv.ContactID)
	if err != nil {
		return err
	}
	if compliance.IsSuppressed(c) {
		// Treat as cold and stop the run.
		return e.repo.StopRunsByConversation(ctx, conv.ID)
	}
	if c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		return nil
	}

	sp, _ := e.sender.Get(ctx, conv.UserID)
	if sp == nil {
		sp = &domain.SenderProfile{UserID: conv.UserID, Tone: "formal"}
	}

	// Render conversation history.
	msgs, err := e.conv.ListMessages(ctx, conv.ID)
	if err != nil {
		return err
	}
	var sb strings.Builder
	for _, m := range msgs {
		if m.Status == domain.MessageStatusDraft || m.Status == domain.MessageStatusPendingApproval {
			continue
		}
		who := "YOU"
		if m.Direction == domain.DirectionInbound {
			who = "THEM"
		}
		sb.WriteString(fmt.Sprintf("--- %s ---\nSubject: %s\n%s\n\n", who, derefSubject(&m), derefBody(&m)))
	}

	businessJSON := e.leadCtx.BusinessJSON(ctx, c.BusinessID)
	leadScoreJSON := e.leadCtx.LeadScoreJSON(ctx, conv.UserID, c.BusinessID)
	hasCatalog, catalogName := leadctx.SenderHasCatalog(sp)

	prompt := prompts.BuildOutreachReplyPrompt(prompts.OutreachReplyInput{
		Mode:              "followup",
		SenderProfileJSON: prompts.EncodeJSON(sp),
		ContactJSON:       prompts.EncodeJSON(c),
		BusinessJSON:      businessJSON,
		LeadScoreJSON:     leadScoreJSON,
		ConversationText:  sb.String(),
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
	})
	ctxAI := ai.WithUserID(ctx, conv.UserID)
	raw, _, err := e.ai.CompleteJSON(ctxAI, "outreach_reply", prompt.Prompt, prompt.System, 30*time.Minute)
	if err != nil {
		return err
	}
	var out prompts.OutreachReplyResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if out.Body == "" {
		return errors.New("ai returned empty followup body")
	}

	// Decide auto-send vs queue: require >=1 prior human-sent outbound on
	// this conversation when auto_send=true.
	canAutoSend := step.AutoSend && hasHumanSent(msgs)

	subject := ""
	if conv.Subject != nil {
		subject = "Re: " + strings.TrimPrefix(*conv.Subject, "Re: ")
	}

	if !canAutoSend {
		// Queue as pending_approval. Do NOT send.
		promptV := "outreach_followup_v1"
		_, err := e.conv.CreateMessage(ctx, domain.Message{
			ConversationID:  conv.ID,
			Direction:       domain.DirectionOutbound,
			ChannelType:     conv.ChannelType,
			Subject:         &subject,
			BodyText:        &out.Body,
			AIGenerated:     true,
			AIPromptVersion: &promptV,
			Status:          domain.MessageStatusPendingApproval,
			SequenceStepID:  &step.ID,
		})
		return err
	}

	// Auto-send path.
	uc, err := e.channels.Get(ctx, conv.UserID, conv.ChannelID)
	if err != nil {
		return err
	}
	ch, err := e.registry.Build(ctx, *uc)
	if err != nil {
		return err
	}
	unsubURL, err := compliance.BuildUnsubscribeURL(e.publicAPIURL, *uc, c.ID, uuid.Nil)
	if err != nil {
		return err
	}
	body := out.Body + compliance.FooterText(sp.PhysicalAddress, unsubURL)
	sendReq := channel.SendRequest{
		To:             *c.PrimaryEmail,
		Subject:        subject,
		BodyText:       body,
		UnsubscribeURL: unsubURL,
	}
	if conv.ExternalThreadID != nil {
		sendReq.ThreadID = *conv.ExternalThreadID
	}
	res, err := ch.Send(ctx, sendReq)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	promptV := "outreach_followup_v1"
	_, err = e.conv.CreateMessage(ctx, domain.Message{
		ConversationID:  conv.ID,
		Direction:       domain.DirectionOutbound,
		ChannelType:     conv.ChannelType,
		ExternalID:      &res.ExternalMessageID,
		Subject:         &subject,
		BodyText:        &out.Body,
		AIGenerated:     true,
		AIPromptVersion: &promptV,
		Status:          domain.MessageStatusSent,
		SequenceStepID:  &step.ID,
		SentAt:          &now,
	})
	if err != nil {
		return err
	}
	_ = e.conv.UpdateLast(ctx, conv.ID, domain.DirectionOutbound, now, false)
	return nil
}

func hasHumanSent(msgs []domain.Message) bool {
	for _, m := range msgs {
		if m.Direction == domain.DirectionOutbound &&
			m.Status == domain.MessageStatusSent &&
			!m.AIGenerated {
			return true
		}
		// Also count AI-generated messages that the user manually approved+sent.
		// We can't distinguish those from auto-sent, so be lenient: any sent
		// outbound counts as evidence the user has "blessed" this conversation.
		if m.Direction == domain.DirectionOutbound && m.Status == domain.MessageStatusSent {
			return true
		}
	}
	return false
}

func derefSubject(m *domain.Message) string {
	if m == nil || m.Subject == nil {
		return ""
	}
	return *m.Subject
}
func derefBody(m *domain.Message) string {
	if m == nil {
		return ""
	}
	if m.BodyText != nil {
		return *m.BodyText
	}
	if m.BodyHTML != nil {
		return *m.BodyHTML
	}
	return ""
}

// loadConversation is a thin helper that uses GetByID since the engine
// doesn't have a userID up-front.
func (e *Engine) loadConversation(ctx context.Context, id uuid.UUID) (*domain.Conversation, error) {
	var c domain.Conversation
	err := e.pool.QueryRow(ctx,
		`SELECT id, user_id, contact_id, channel_id, campaign_id, channel_type, subject,
		        external_thread_id, automation, status, last_message_at, last_direction,
		        unread, created_at, updated_at
		 FROM conversations WHERE id = $1`, id,
	).Scan(&c.ID, &c.UserID, &c.ContactID, &c.ChannelID, &c.CampaignID, &c.ChannelType, &c.Subject,
		&c.ExternalThreadID, &c.Automation, &c.Status, &c.LastMessageAt, &c.LastDirection,
		&c.Unread, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
