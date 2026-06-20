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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/internalsched"
	"github.com/batuhan/marketintel/internal/outreach/leadctx"
	"github.com/batuhan/marketintel/internal/outreach/replyguidance"
	"github.com/batuhan/marketintel/internal/outreach/sender"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// TaskNotifier is the minimal surface the engine needs to record a
// notify_user action as a task row. Bound to crm.Repository.CreateTask in
// main.go (a local interface avoids a sequence→crm import cycle).
type TaskNotifier interface {
	CreateTask(ctx context.Context, t domain.Task) (*domain.Task, error)
}

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
	tasks        TaskNotifier
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
	tasks TaskNotifier,
	registry *channel.Registry,
	aiRouter *ai.Router,
	leadCtxLoader *leadctx.Loader,
	pool *pgxpool.Pool,
	publicAPIURL string,
) *Engine {
	return &Engine{
		repo: repo, conv: convRepo, contacts: contacts, channels: channels,
		sender: senderRepo, tasks: tasks, registry: registry, ai: aiRouter,
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

	// Response-aware branch: the moment the recipient replies, the no-reply
	// cadence stops and we react. An intent tag (unsubscribe / out-of-office /
	// wrong-contact) can override the sentiment branch; out-of-office snoozes
	// the run (kept active) instead of completing it.
	if conv.LastDirection != nil && *conv.LastDirection == domain.DirectionInbound {
		// Never branch on an unclassified reply (poller crash mid-persist or AI
		// outage leaves messages.sentiment NULL): wait a tick — the poller's
		// reclassify pass will fill it in. Branching blind would route an
		// unsubscribe reply into a friendly auto-draft.
		classified, err := e.lastInboundClassified(ctx, conv.ID)
		if err != nil {
			return fmt.Errorf("classification check: %w", err)
		}
		if !classified {
			return e.repo.BumpRun(ctx, run.ID, time.Now().Add(60*time.Second))
		}
		snoozeUntil, err := e.handleReplyBranch(ctx, conv)
		if err != nil {
			return fmt.Errorf("reply branch: %w", err)
		}
		if snoozeUntil != nil {
			return e.repo.BumpRun(ctx, run.ID, *snoozeUntil)
		}
		return e.repo.CompleteRun(ctx, run.ID, domain.SequenceRunStoppedOnReply)
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

// oooSnoozeWindow is how long an out-of-office reply pauses the run before it
// resumes (and, with the OOO tag consumed, falls through to the normal branch).
const oooSnoozeWindow = 3 * 24 * time.Hour

// handleReplyBranch reacts to an inbound reply. An intent tag can override the
// sentiment branch (precedence: unsubscribe > out_of_office > wrong_contact);
// otherwise it falls through to the campaign's positive/negative branch. Returns
// a non-nil snoozeUntil when the run should be bumped (kept active) rather than
// completed — used for out-of-office.
func (e *Engine) handleReplyBranch(ctx context.Context, conv *domain.Conversation) (*time.Time, error) {
	tags, err := e.loadConvTags(ctx, conv.ID)
	if err != nil {
		// Don't route blind: a failed tag read could turn an unsubscribe reply
		// into a friendly auto-draft. Error out — processOne leaves the run
		// active and it retries next tick.
		return nil, fmt.Errorf("load tags: %w", err)
	}
	switch ResolveIntentRouting(tags) {
	case RouteSuppress:
		// Compliance: unsubscribe intent → suppress the contact + mark cold.
		// (The poller also suppresses at ingestion; both are idempotent.)
		if err := e.contacts.MarkUnsubscribed(ctx, conv.ContactID, "reply_intent_unsubscribe"); err != nil {
			return nil, err
		}
		_, err := e.pool.Exec(ctx,
			`UPDATE campaign_contacts SET status='cold' WHERE conversation_id=$1`, conv.ID)
		return nil, err
	case RouteSnooze:
		// Out-of-office: don't draft a reply to a bot. Consume the AI-set OOO
		// tag (manual tags are never touched) so the next pass — after the
		// window — falls through to the normal branch, and keep the run active.
		// Surface a task so the user knows why the thread went quiet.
		if _, err := e.pool.Exec(ctx,
			`DELETE FROM conversation_tags WHERE conversation_id=$1 AND tag=$2 AND source='ai'`,
			conv.ID, domain.TagOutOfOffice); err != nil {
			return nil, err
		}
		_ = e.notifyReply(ctx, conv, "Out-of-office auto-reply — follow-up snoozed 3 days")
		until := time.Now().UTC().Add(oooSnoozeWindow)
		return &until, nil
	case RouteNotify:
		// Wrong contact: surface a task, no auto-draft.
		return nil, e.notifyReply(ctx, conv, "Wrong contact — needs your attention")
	}

	// Sentiment branch: negative → on_negative (default mark cold);
	// positive/neutral → on_positive (default auto-draft a reply for approval).
	onPos, onNeg, sentiment := e.replyPolicy(ctx, conv)
	branch, action := ResolveReplyBranch(sentiment, onPos, onNeg)

	if branch == "negative" {
		switch action {
		case domain.OnNegativeNotify:
			return nil, e.notifyReply(ctx, conv, "Lead replied — declined; review")
		default: // mark_cold
			_, err := e.pool.Exec(ctx,
				`UPDATE campaign_contacts SET status='cold' WHERE conversation_id=$1`, conv.ID)
			return nil, err
		}
	}
	// positive or neutral → engage
	switch action {
	case domain.OnPositiveNotify:
		return nil, e.notifyReply(ctx, conv, "Lead replied — needs your attention")
	default: // auto_draft_reply (auto_send falls back to draft-for-approval in v1)
		return nil, e.draftReplyForApproval(ctx, conv)
	}
}

// loadConvTags returns the intent tags currently on a conversation. Errors are
// returned (not swallowed) — routing on a silently-empty tag set could misroute
// compliance-critical replies.
func (e *Engine) loadConvTags(ctx context.Context, convID uuid.UUID) ([]string, error) {
	rows, err := e.pool.Query(ctx, `SELECT tag FROM conversation_tags WHERE conversation_id=$1`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, rows.Err()
}

// lastInboundClassified reports whether the newest inbound message on the
// conversation has a persisted sentiment (i.e. classification committed).
func (e *Engine) lastInboundClassified(ctx context.Context, convID uuid.UUID) (bool, error) {
	var classified *bool
	err := e.pool.QueryRow(ctx,
		`SELECT sentiment IS NOT NULL FROM messages
		  WHERE conversation_id=$1 AND direction='in'
		  ORDER BY created_at DESC, id DESC LIMIT 1`, convID,
	).Scan(&classified)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// last_direction says inbound but no inbound row — treat as not ready.
			return false, nil
		}
		return false, err
	}
	return classified != nil && *classified, nil
}

// ResolveReplyBranch is the pure decision: a negative reply takes the negative
// branch/action, anything else takes the positive branch/action. Shared with
// the simulation so the Lab mirrors production routing.
func ResolveReplyBranch(sentiment, onPositive, onNegative string) (branch, action string) {
	if sentiment == domain.SentimentNegative {
		return "negative", onNegative
	}
	return "positive", onPositive
}

// replyPolicy resolves the branch actions + the stored inbound sentiment for a
// conversation. Falls back to the system defaults for non-campaign threads.
func (e *Engine) replyPolicy(ctx context.Context, conv *domain.Conversation) (onPos, onNeg, sentiment string) {
	onPos, onNeg = domain.OnPositiveAutoDraftReply, domain.OnNegativeMarkCold
	if conv.CampaignID != nil {
		var p, n string
		if err := e.pool.QueryRow(ctx,
			`SELECT COALESCE(on_positive_action,''), COALESCE(on_negative_action,'') FROM campaigns WHERE id=$1`,
			*conv.CampaignID,
		).Scan(&p, &n); err == nil {
			if p != "" {
				onPos = p
			}
			if n != "" {
				onNeg = n
			}
		}
	}
	sentiment = domain.SentimentNeutral
	_ = e.pool.QueryRow(ctx,
		`SELECT COALESCE(last_inbound_sentiment, 'neutral') FROM conversations WHERE id=$1`, conv.ID,
	).Scan(&sentiment)
	return onPos, onNeg, sentiment
}

// notifyReply records a task so the user can take over a replied lead.
func (e *Engine) notifyReply(ctx context.Context, conv *domain.Conversation, title string) error {
	if e.tasks == nil {
		return nil
	}
	contactID := conv.ContactID
	convID := conv.ID
	_, err := e.tasks.CreateTask(ctx, domain.Task{
		UserID:         conv.UserID,
		ContactID:      &contactID,
		ConversationID: &convID,
		Title:          title,
		Body:           "Reply received — automation paused for your input.",
	})
	return err
}

// draftReplyForApproval drafts a context-aware reply to the recipient's last
// message and queues it as pending_approval (reuses the followup drafting +
// guard). It does not send.
func (e *Engine) draftReplyForApproval(ctx context.Context, conv *domain.Conversation) error {
	c, err := e.contacts.Get(ctx, conv.UserID, conv.ContactID)
	if err != nil {
		return err
	}
	if compliance.IsSuppressed(c) {
		// Never draft to a suppressed contact, even for approval — the reply
		// branch can race a keyword/one-click unsubscribe.
		slog.Info("auto-draft skipped: contact suppressed", "conversation_id", conv.ID)
		return nil
	}
	if c == nil || c.PrimaryEmail == nil || *c.PrimaryEmail == "" {
		// Don't silently swallow the reply — surface it so the user can act.
		return e.notifyReply(ctx, conv, "Lead replied — no email on contact, reply manually")
	}
	sp := e.brandForConv(ctx, conv)

	msgs, err := e.conv.ListMessages(ctx, conv.ID)
	if err != nil {
		return err
	}
	// Replace any stale pending draft (engine re-entry, earlier manual draft) —
	// two pending_approval rows confuse the compose pre-fill.
	if err := e.conv.DeletePendingDrafts(ctx, conv.ID); err != nil {
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

	guidance := ""
	if sp != nil {
		guidance = replyguidance.Load(ctx, e.pool, conv.ID, conv.CampaignID, sp.ID)
	}
	out, err := e.followupWithGuard(ctx, conv.UserID, prompts.OutreachReplyInput{
		Mode:              "reply",
		SenderProfileJSON: prompts.EncodeJSON(sp),
		ContactJSON:       prompts.EncodeJSON(c),
		BusinessJSON:      businessJSON,
		LeadScoreJSON:     leadScoreJSON,
		ConversationText:  sb.String(),
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
		Guidance:          guidance,
	})
	if err != nil {
		return err
	}
	subject := ""
	if conv.Subject != nil {
		subject = "Re: " + strings.TrimPrefix(*conv.Subject, "Re: ")
	}
	promptV := "outreach_reply_v1"
	_, err = e.conv.CreateMessage(ctx, domain.Message{
		ConversationID:  conv.ID,
		Direction:       domain.DirectionOutbound,
		ChannelType:     conv.ChannelType,
		Subject:         &subject,
		BodyText:        &out.Body,
		AIGenerated:     true,
		AIPromptVersion: &promptV,
		Status:          domain.MessageStatusPendingApproval,
	})
	return err
}

// evaluateTrigger checks whether the step's trigger condition is met against
// the current conversation state. Returns (match, err).
func (e *Engine) evaluateTrigger(ctx context.Context, step *domain.SequenceStep, conv *domain.Conversation) (bool, error) {
	lastMessageAt := time.Time{}
	if conv.LastMessageAt != nil {
		lastMessageAt = *conv.LastMessageAt
	}

	// Cadence steps only run while there's no reply — an inbound reply is
	// intercepted upstream in processOne (the campaign reply branch) before we
	// ever get here, so the reply-based triggers are unreachable. Pass nil for
	// the positive-reply signal; EvaluateTriggerPure handles no_reply timing.
	return EvaluateTriggerPure(
		step.Trigger, step.WaitDays, conv.LastDirection, lastMessageAt, time.Now().UTC(), nil,
	), nil
}

// fireAction executes the step's action against the conversation.
func (e *Engine) fireAction(ctx context.Context, step *domain.SequenceStep, conv *domain.Conversation) error {
	switch step.Action {
	case domain.SequenceActionSendMessage:
		return e.actionSendMessage(ctx, step, conv)
	case domain.SequenceActionMarkCold:
		// Cadence exhaustion makes the contact cold FOR THIS CAMPAIGN — it is
		// not a legal opt-out. MarkUnsubscribed (contacts.unsubscribed_at) is
		// reserved strictly for compliance events (one-click, reply keyword,
		// unsubscribe intent); overloading it here permanently blocked
		// non-repliers from every future campaign.
		_, err := e.pool.Exec(ctx,
			`UPDATE campaign_contacts SET status='cold' WHERE conversation_id=$1 AND status NOT IN ('cold','skipped','failed')`,
			conv.ID)
		return err
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
		// Record a task row so the notification surfaces in the Email Group
		// view (tasks filtered by the group's conversations).
		if e.tasks == nil {
			slog.Warn("sequence notify_user skipped: no task notifier configured", "conversation_id", conv.ID)
			return nil
		}
		title := "Follow-up needs your attention"
		if step.PromptOverride != nil && *step.PromptOverride != "" {
			title = *step.PromptOverride
		}
		contactID := conv.ContactID
		convID := conv.ID
		_, err := e.tasks.CreateTask(ctx, domain.Task{
			UserID:         conv.UserID,
			ContactID:      &contactID,
			ConversationID: &convID,
			Title:          title,
			Body:           fmt.Sprintf("Sequence step %d fired notify_user.", step.StepNumber),
		})
		if err != nil {
			return err
		}
		slog.Info("sequence notify_user task created", "conversation_id", conv.ID, "step", step.StepNumber)
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

	sp := e.brandForConv(ctx, conv)

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

	in := prompts.OutreachReplyInput{
		Mode:              "followup",
		SenderProfileJSON: prompts.EncodeJSON(sp),
		ContactJSON:       prompts.EncodeJSON(c),
		BusinessJSON:      businessJSON,
		LeadScoreJSON:     leadScoreJSON,
		ConversationText:  sb.String(),
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
	}
	out, err := e.followupWithGuard(ctx, conv.UserID, in)
	if err != nil {
		return err
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

// followupWithGuard runs the followup AI completion, validates the body
// with the post-gen guard (Followup mode tolerates "circle back"/"touch
// base"), and retries once with StricterRetry=true on violation. Falls
// back to first attempt on retry failure so an off-brand followup is
// preferred over no followup.
func (e *Engine) followupWithGuard(ctx context.Context, userID uuid.UUID, in prompts.OutreachReplyInput) (*prompts.OutreachReplyResult, error) {
	first, err := e.followupOnce(ctx, userID, in)
	if err != nil {
		return nil, err
	}
	// Pick the guard mode from the draft mode — replies have a different word
	// cap than followups; validating a reply against the followup cap caused
	// spurious rejections the conversation service would never produce.
	mode := prompts.DraftModeFollowup
	if in.Mode == "reply" {
		mode = prompts.DraftModeReply
	}
	violations := prompts.ValidateOutreachBody(first.Body, in.HasCatalog, mode)
	if len(violations) == 0 {
		return first, nil
	}
	slog.Warn("sequence followup: guard violations, retrying",
		"violations", violations, "mode", in.Mode,
	)
	in.StricterRetry = true
	second, err := e.followupOnce(ctx, userID, in)
	if err != nil {
		return first, nil
	}
	return second, nil
}

func (e *Engine) followupOnce(ctx context.Context, userID uuid.UUID, in prompts.OutreachReplyInput) (*prompts.OutreachReplyResult, error) {
	prompt := prompts.BuildOutreachReplyPrompt(in)
	ctxAI := ai.WithUserID(ctx, userID)
	raw, _, err := e.ai.CompleteJSON(ctxAI, "outreach_reply", prompt.Prompt, prompt.System, 30*time.Minute)
	if err != nil {
		return nil, err
	}
	var out prompts.OutreachReplyResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Body == "" {
		return nil, errors.New("ai returned empty followup body")
	}
	return &out, nil
}

// brandForConv resolves the brand a followup should send/draft as. If the
// conversation belongs to a campaign (Email Group) with an assigned brand,
// use it; otherwise the user's Default brand. Reads campaigns.sender_profile_id
// directly via the pool to avoid a sequence→campaign import cycle. Never nil.
func (e *Engine) brandForConv(ctx context.Context, conv *domain.Conversation) *domain.SenderProfile {
	if conv != nil && conv.CampaignID != nil {
		var profileID *uuid.UUID
		if err := e.pool.QueryRow(ctx,
			`SELECT sender_profile_id FROM campaigns WHERE id = $1`, *conv.CampaignID,
		).Scan(&profileID); err == nil && profileID != nil {
			if sp, err := e.sender.GetByID(ctx, conv.UserID, *profileID); err == nil && sp != nil {
				return sp
			}
		}
	}
	if sp, err := e.sender.DefaultForUser(ctx, conv.UserID); err == nil && sp != nil {
		return sp
	}
	return &domain.SenderProfile{UserID: conv.UserID, Tone: "formal"}
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
