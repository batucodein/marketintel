package simulation

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
	"github.com/batuhan/marketintel/internal/outreach/events"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// agent.go is the simulation's conversational tool-calling agent — the same
// experience as the live group dock, scoped to a simulation. It reuses the
// shared agentcore.RunTurn loop + the shared assistant prompt, with sim-scoped
// read tools (agent_tools.go) and confirm-gated writes over the sim's own state.

const simAssistantHistoryLimit = 20

var simEditScopes = map[string]bool{"cold": true, "reply": true, "reply_positive": true, "reply_negative": true, "all": true}

// AssistantMessage is one chat message in a simulation's agent thread.
type AssistantMessage struct {
	ID             uuid.UUID                `json:"id"`
	Role           string                   `json:"role"`
	Content        string                   `json:"content"`
	ProposedAction *prompts.AssistantAction `json:"proposed_action,omitempty"`
	Status         string                   `json:"status"`
	CreatedAt      time.Time                `json:"created_at"`
}

// AssistantHistory returns the persisted chat thread (oldest first).
func (s *Service) AssistantHistory(ctx context.Context, userID, simID uuid.UUID) ([]AssistantMessage, error) {
	if _, err := s.repo.Get(ctx, userID, simID); err != nil {
		return nil, err
	}
	return s.repo.ListAssistantMessages(ctx, simID, 200)
}

// AssistantSend runs one agent turn over the simulation's live state, streaming
// each tool it calls as an "assistant_thinking" event so the dock shows status.
func (s *Service) AssistantSend(ctx context.Context, userID, simID uuid.UUID, text string) (*AssistantMessage, error) {
	sim, err := s.repo.Get(ctx, userID, simID)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("message is required")
	}
	brand, err := s.sender.GetByID(ctx, userID, *sim.SenderProfileID)
	if err != nil {
		return nil, fmt.Errorf("brand not found: %w", err)
	}
	if _, _, err := s.repo.AddAssistantMessage(ctx, simID, "user", text, nil, "sent"); err != nil {
		return nil, fmt.Errorf("store user message: %w", err)
	}
	onTool := func(name string) {
		if s.broker == nil {
			return
		}
		sid := simID
		s.broker.Publish(events.Event{Kind: events.KindSimulationProgress, UserID: userID,
			Data: map[string]any{"simulation_id": sid.String(), "action": "assistant_thinking", "tool": name}})
	}

	base := prompts.AgentTurnInput{
		BrandPositioning: simBrand(brand),
		Counts:           simCounts(sim),
		Playbook:         simPlaybookText(sim),
		History:          s.renderSimChatHistory(ctx, simID),
		UserMessage:      text,
	}
	actx := ai.WithUserID(ctx, userID)
	reply, action := agentcore.RunTurn(actx, s.ai, base,
		func(name string, args json.RawMessage) string {
			return s.dispatchSimTool(ctx, userID, sim, brand, name, args)
		},
		validateSimAction, onTool, "sim:"+simID.String())

	status := "sent"
	var actionJSON json.RawMessage
	if action != nil {
		status = "proposed"
		b, _ := json.Marshal(action)
		actionJSON = b
	}
	id, created, err := s.repo.AddAssistantMessage(ctx, simID, "assistant", reply, actionJSON, status)
	if err != nil {
		return nil, fmt.Errorf("store assistant message: %w", err)
	}
	return &AssistantMessage{ID: id, Role: "assistant", Content: reply, ProposedAction: action, Status: status, CreatedAt: created}, nil
}

// validateSimAction returns the action only if well-formed (same vocabulary as
// the group agent), else nil.
func validateSimAction(a *prompts.AssistantAction) *prompts.AssistantAction {
	if a == nil {
		return nil
	}
	switch a.Type {
	case "edit_drafts":
		if simEditScopes[a.Scope] && strings.TrimSpace(a.Instruction) != "" {
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

// AssistantConfirm executes the action proposed on a message.
func (s *Service) AssistantConfirm(ctx context.Context, userID, simID, msgID uuid.UUID, progress func(done, total int)) (int, int, error) {
	sim, err := s.repo.Get(ctx, userID, simID)
	if err != nil {
		return 0, 0, err
	}
	raw, status, err := s.repo.GetAssistantProposal(ctx, simID, msgID)
	if err != nil {
		return 0, 0, err
	}
	if status != "proposed" || len(raw) == 0 {
		return 0, 0, errors.New("no pending action on this message")
	}
	var action prompts.AssistantAction
	if err := json.Unmarshal(raw, &action); err != nil {
		return 0, 0, err
	}

	var (
		updated, failed int
		result          string
	)
	switch action.Type {
	case "edit_drafts":
		brand, berr := s.sender.GetByID(ctx, userID, *sim.SenderProfileID)
		if berr != nil {
			return 0, 0, berr
		}
		updated, failed = s.execEditSimDrafts(ctx, userID, sim, brand, action, progress)
		result = draftResultSim(updated, failed)
	case "add_playbook":
		pb := sim.Playbook
		if pb == nil {
			pb = map[string]string{}
		}
		pb[action.Tag] = strings.TrimSpace(action.Instruction)
		if err := s.repo.SetPlaybook(ctx, simID, pb); err != nil {
			return 0, 0, err
		}
		result = fmt.Sprintf("Added to the playbook — for “%s” replies: %s", action.Tag, strings.TrimSpace(action.Instruction))
	case "remove_playbook":
		pb := sim.Playbook
		delete(pb, action.Tag)
		if err := s.repo.SetPlaybook(ctx, simID, pb); err != nil {
			return 0, 0, err
		}
		result = fmt.Sprintf("Removed the “%s” rule from the playbook.", action.Tag)
	default:
		return 0, 0, errors.New("unknown action")
	}

	_ = s.repo.SetAssistantStatus(ctx, simID, msgID, "applied")
	_, _, _ = s.repo.AddAssistantMessage(ctx, simID, "assistant", result, nil, "sent")
	return updated, failed, nil
}

// AssistantDismiss marks a proposal declined.
func (s *Service) AssistantDismiss(ctx context.Context, userID, simID, msgID uuid.UUID) error {
	if _, err := s.repo.Get(ctx, userID, simID); err != nil {
		return err
	}
	return s.repo.SetAssistantStatus(ctx, simID, msgID, "dismissed")
}

// execEditSimDrafts rewrites every pending draft matching the cohort scope.
func (s *Service) execEditSimDrafts(ctx context.Context, userID uuid.UUID, sim *Simulation, brand *domain.SenderProfile, a prompts.AssistantAction, progress func(done, total int)) (int, int) {
	instr := strings.TrimSpace(a.Instruction)
	var targets []*Lead
	for i := range sim.Leads {
		l := &sim.Leads[i]
		if l.State != stateAwaiting || l.PendingDraft == nil {
			continue
		}
		if simScopeMatches(a.Scope, l) {
			targets = append(targets, l)
		}
	}
	total := len(targets)
	updated, failed := 0, 0
	for i, l := range targets {
		s.refinePending(ctx, userID, brand, l, instr)
		if err := s.repo.UpdateLead(ctx, *l); err != nil {
			failed++
		} else {
			updated++
		}
		if progress != nil {
			progress(i+1, total)
		}
	}
	return updated, failed
}

func simScopeMatches(scope string, l *Lead) bool {
	pd := l.PendingDraft
	switch scope {
	case "all":
		return true
	case "cold":
		return pd.Kind == "cold"
	case "reply":
		return pd.Kind == "reply"
	case "reply_positive":
		return pd.Kind == "reply" && l.LastSentiment != nil && *l.LastSentiment == domain.SentimentPositive
	case "reply_negative":
		return pd.Kind == "reply" && l.LastSentiment != nil && *l.LastSentiment == domain.SentimentNegative
	}
	return false
}

func draftResultSim(updated, failed int) string {
	if updated == 0 && failed == 0 {
		return "No matching drafts to update."
	}
	r := fmt.Sprintf("Done — %d draft(s) updated.", updated)
	if failed > 0 {
		r += fmt.Sprintf(" (%d couldn't be updated.)", failed)
	}
	return r
}

func (s *Service) renderSimChatHistory(ctx context.Context, simID uuid.UUID) string {
	msgs, err := s.repo.ListAssistantMessages(ctx, simID, simAssistantHistoryLimit)
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
