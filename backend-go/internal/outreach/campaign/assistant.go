package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/agentcore"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

const assistantHistoryLimit = 20

var editScopes = map[string]bool{"cold": true, "reply": true, "reply_positive": true, "reply_negative": true, "all": true}

// AssistantMessage is one chat message in the group's draft-assistant thread.
type AssistantMessage struct {
	ID             uuid.UUID                `json:"id"`
	Role           string                   `json:"role"`
	Content        string                   `json:"content"`
	ProposedAction *prompts.AssistantAction `json:"proposed_action,omitempty"`
	Status         string                   `json:"status"`
	CreatedAt      time.Time                `json:"created_at"`
}

// AssistantHistory returns the persisted chat thread (oldest first).
func (s *Service) AssistantHistory(ctx context.Context, userID, campaignID uuid.UUID) ([]AssistantMessage, error) {
	if _, err := s.repo.Get(ctx, userID, campaignID); err != nil {
		return nil, err
	}
	return s.loadAssistantMessages(ctx, campaignID, 200)
}

// AssistantSend records the user's message, runs the tool-calling agent loop
// (read-only tools fetch exactly what the answer needs, on demand), then
// persists the reply — with a proposed action when the agent wants to change
// something. onTool, when set, is invoked with each tool name as it runs so the
// caller can stream live "thinking" status to the UI.
//
// The loop is deliberately provider-agnostic: it only uses prompt+system
// strings via ai.CompleteJSON and parses JSON out — no provider's native
// function-calling API. Switching providers is a task→provider remap, nothing
// here changes.
func (s *Service) AssistantSend(ctx context.Context, userID, campaignID uuid.UUID, text string, onTool func(name string)) (*AssistantMessage, error) {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("message is required")
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO campaign_assistant_messages (campaign_id, role, content, status) VALUES ($1,'user',$2,'sent')`,
		campaignID, text); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}

	// Always-on (cheap) context; everything else is fetched via tools.
	base := prompts.AgentTurnInput{
		BrandPositioning: s.assistantBrandContext(ctx, camp),
		Counts:           s.renderCounts(ctx, campaignID),
		Playbook:         s.renderPlaybook(ctx, campaignID),
		History:          s.renderAssistantHistory(ctx, campaignID),
		UserMessage:      text,
	}

	actx := ai.WithUserID(ctx, userID)
	reply, action := agentcore.RunTurn(actx, s.ai, base,
		func(name string, args json.RawMessage) string {
			return s.dispatchAgentTool(ctx, userID, camp, &prompts.AgentToolCall{Name: name, Args: args})
		},
		validateAction, onTool, "campaign:"+campaignID.String())

	status := "sent"
	var actionJSON any
	if action != nil {
		status = "proposed"
		b, _ := json.Marshal(action)
		actionJSON = b
	}
	var id uuid.UUID
	var created time.Time
	if err := s.pool.QueryRow(ctx,
		`INSERT INTO campaign_assistant_messages (campaign_id, role, content, proposed_action, status)
		 VALUES ($1,'assistant',$2,$3,$4) RETURNING id, created_at`,
		campaignID, reply, actionJSON, status,
	).Scan(&id, &created); err != nil {
		return nil, fmt.Errorf("store assistant message: %w", err)
	}
	return &AssistantMessage{ID: id, Role: "assistant", Content: reply, ProposedAction: action, Status: status, CreatedAt: created}, nil
}

// validateAction returns the action only if it's well-formed, else nil (plain reply).
func validateAction(a *prompts.AssistantAction) *prompts.AssistantAction {
	if a == nil {
		return nil
	}
	switch a.Type {
	case "edit_drafts":
		if editScopes[a.Scope] && strings.TrimSpace(a.Instruction) != "" {
			return a
		}
	case "add_playbook":
		if domain.IsValidIntentTag(a.Tag) && strings.TrimSpace(a.Instruction) != "" {
			return a
		}
	case "remove_playbook":
		if domain.IsValidIntentTag(a.Tag) {
			return a
		}
	}
	return nil
}

// AssistantConfirm executes the action proposed on an assistant message.
func (s *Service) AssistantConfirm(ctx context.Context, userID, campaignID, messageID uuid.UUID, progress func(done, total int)) (int, int, error) {
	if _, err := s.repo.Get(ctx, userID, campaignID); err != nil {
		return 0, 0, err
	}
	var actionRaw []byte
	var status string
	if err := s.pool.QueryRow(ctx,
		`SELECT proposed_action, status FROM campaign_assistant_messages
		  WHERE id=$1 AND campaign_id=$2 AND role='assistant'`, messageID, campaignID,
	).Scan(&actionRaw, &status); err != nil {
		return 0, 0, err
	}
	if status != "proposed" || len(actionRaw) == 0 {
		return 0, 0, errors.New("no pending action on this message")
	}
	var action prompts.AssistantAction
	if err := json.Unmarshal(actionRaw, &action); err != nil {
		return 0, 0, err
	}

	var (
		updated, failed int
		result          string
		err             error
	)
	switch action.Type {
	case "edit_drafts":
		updated, failed, result, err = s.execEditDrafts(ctx, userID, campaignID, action, progress)
	case "add_playbook":
		_, err = s.pool.Exec(ctx,
			`INSERT INTO campaign_tag_guidance (campaign_id, tag, instruction) VALUES ($1,$2,$3)
			 ON CONFLICT (campaign_id, tag) DO UPDATE SET instruction=EXCLUDED.instruction, updated_at=now()`,
			campaignID, action.Tag, strings.TrimSpace(action.Instruction))
		result = fmt.Sprintf("Added to the playbook — for “%s” replies: %s", action.Tag, strings.TrimSpace(action.Instruction))
	case "remove_playbook":
		_, err = s.pool.Exec(ctx,
			`DELETE FROM campaign_tag_guidance WHERE campaign_id=$1 AND tag=$2`, campaignID, action.Tag)
		result = fmt.Sprintf("Removed the “%s” rule from the playbook (future replies only).", action.Tag)
	default:
		return 0, 0, errors.New("unknown action")
	}
	if err != nil {
		return 0, 0, err
	}

	_, _ = s.pool.Exec(ctx, `UPDATE campaign_assistant_messages SET status='applied' WHERE id=$1`, messageID)
	_, _ = s.pool.Exec(ctx,
		`INSERT INTO campaign_assistant_messages (campaign_id, role, content, status) VALUES ($1,'assistant',$2,'sent')`,
		campaignID, result)
	return updated, failed, nil
}

func (s *Service) execEditDrafts(ctx context.Context, userID, campaignID uuid.UUID, a prompts.AssistantAction, progress func(done, total int)) (int, int, string, error) {
	instr := strings.TrimSpace(a.Instruction)
	switch a.Scope {
	case "cold":
		u, f, err := s.rewriteColdDrafts(ctx, userID, campaignID, instr, progress)
		return u, f, draftResult("cold opener", u, f), err
	case "reply", "reply_positive", "reply_negative":
		sentiment := ""
		if a.Scope == "reply_positive" {
			sentiment = domain.SentimentPositive
		} else if a.Scope == "reply_negative" {
			sentiment = domain.SentimentNegative
		}
		u, f, err := s.rewriteReplyDrafts(ctx, userID, campaignID, instr, sentiment, progress)
		return u, f, draftResult("reply", u, f), err
	case "all":
		u1, f1, err := s.rewriteColdDrafts(ctx, userID, campaignID, instr, progress)
		if err != nil {
			return u1, f1, "", err
		}
		u2, f2, err := s.rewriteReplyDrafts(ctx, userID, campaignID, instr, "", progress)
		return u1 + u2, f1 + f2, draftResult("", u1+u2, f1+f2), err
	}
	return 0, 0, "", errors.New("unknown scope")
}

func draftResult(kind string, updated, failed int) string {
	label := "draft"
	if kind != "" {
		label = kind + " draft"
	}
	if updated == 0 && failed == 0 {
		return "No matching drafts to update."
	}
	r := fmt.Sprintf("Done — %d %s%s updated.", updated, label, plural(updated))
	if failed > 0 {
		r += fmt.Sprintf(" (%d couldn't be updated.)", failed)
	}
	return r
}

// AssistantDismiss marks a proposal declined.
func (s *Service) AssistantDismiss(ctx context.Context, userID, campaignID, messageID uuid.UUID) error {
	if _, err := s.repo.Get(ctx, userID, campaignID); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx,
		`UPDATE campaign_assistant_messages SET status='dismissed'
		  WHERE id=$1 AND campaign_id=$2 AND status='proposed'`, messageID, campaignID)
	return err
}

// --- context rendering (all bounded) ---

func (s *Service) loadAssistantMessages(ctx context.Context, campaignID uuid.UUID, limit int) ([]AssistantMessage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, role, content, proposed_action, status, created_at FROM (
		   SELECT id, role, content, proposed_action, status, created_at
		     FROM campaign_assistant_messages WHERE campaign_id=$1
		     ORDER BY created_at DESC LIMIT $2
		 ) recent ORDER BY created_at ASC`, campaignID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssistantMessage{}
	for rows.Next() {
		var m AssistantMessage
		var actionRaw []byte
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &actionRaw, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		if len(actionRaw) > 0 {
			var a prompts.AssistantAction
			if json.Unmarshal(actionRaw, &a) == nil {
				m.ProposedAction = &a
			}
		}
		out = append(out, m)
	}
	return out, nil
}

// renderCounts returns authoritative tallies (computed via COUNT, not by the
// LLM eyeballing a truncated draft list) so "how many" answers are exact.
func (s *Service) renderCounts(ctx context.Context, campaignID uuid.UUID) string {
	var cold, reply, pos, neg, neu int
	_ = s.pool.QueryRow(ctx,
		`SELECT count(*) FROM campaign_contacts
		  WHERE campaign_id=$1 AND status='drafted' AND draft_message_id IS NOT NULL`, campaignID,
	).Scan(&cold)
	_ = s.pool.QueryRow(ctx,
		`SELECT count(DISTINCT cv.id) FROM conversations cv
		   JOIN messages m ON m.conversation_id=cv.id AND m.direction='out' AND m.status='pending_approval'
		  WHERE cv.campaign_id=$1`, campaignID,
	).Scan(&reply)
	_ = s.pool.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE last_inbound_sentiment='positive'),
		  count(*) FILTER (WHERE last_inbound_sentiment='negative'),
		  count(*) FILTER (WHERE last_inbound_sentiment='neutral')
		FROM conversations WHERE campaign_id=$1 AND last_direction='in'`, campaignID,
	).Scan(&pos, &neg, &neu)
	return fmt.Sprintf(
		"Cold opener drafts awaiting approval: %d\nReply drafts awaiting approval: %d\nReplied so far — positive: %d, negative: %d, neutral: %d",
		cold, reply, pos, neg, neu)
}

func (s *Service) assistantBrandContext(ctx context.Context, camp *domain.Campaign) string {
	sp, _ := s.resolveBrand(ctx, camp)
	var b strings.Builder
	if sp != nil {
		fmt.Fprintf(&b, "Brand: %s. Product: %s.", sp.CompanyName, truncate(sp.ProductDescription, 240))
	}
	goal := camp.Goal
	if o := s.parsePositioning(camp); o != nil && o.Goal != "" {
		goal = o.Goal
	}
	if goal != "" {
		fmt.Fprintf(&b, " Campaign goal: %s.", truncate(goal, 240))
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) renderPlaybook(ctx context.Context, campaignID uuid.UUID) string {
	rows, err := s.pool.Query(ctx,
		`SELECT tag, instruction FROM campaign_tag_guidance WHERE campaign_id=$1 ORDER BY tag`, campaignID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var tag, instr string
		if rows.Scan(&tag, &instr) == nil {
			fmt.Fprintf(&b, "- when reply is %s: %s\n", tag, instr)
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Service) renderAssistantHistory(ctx context.Context, campaignID uuid.UUID) string {
	msgs, err := s.loadAssistantMessages(ctx, campaignID, assistantHistoryLimit)
	if err != nil || len(msgs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, m := range msgs {
		who := "USER"
		if m.Role == "assistant" {
			who = "ASSISTANT"
		}
		fmt.Fprintf(&b, "%s: %s\n", who, m.Content)
	}
	return strings.TrimSpace(b.String())
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
