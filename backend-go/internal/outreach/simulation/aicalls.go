package simulation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// draftCold runs the real cold-draft prompt + guard (one retry on violation),
// returning (subject, body). Mirrors the campaign drafter's behaviour.
func (s *Service) draftCold(ctx context.Context, userID uuid.UUID, in prompts.OutreachDraftInput) (string, string) {
	once := func() (*prompts.OutreachDraftResult, error) {
		p := prompts.BuildOutreachDraftPrompt(in)
		raw, _, err := s.ai.CompleteJSON(ai.WithUserID(ctx, userID), "outreach_draft", p.Prompt, p.System, 0)
		if err != nil {
			return nil, err
		}
		var out prompts.OutreachDraftResult
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	first, err := once()
	if err != nil || first == nil {
		return "Quick note", "(draft failed in simulation)"
	}
	if len(prompts.ValidateOutreachBody(first.Body, in.HasCatalog, prompts.DraftModeCold)) > 0 {
		in.StricterRetry = true
		if second, err := once(); err == nil && second != nil {
			return second.Subject, second.Body
		}
	}
	return first.Subject, first.Body
}

// draftFollowup runs the real reply prompt + guard. The guard mode follows
// in.Mode exactly like the production engine: reply drafts validate against
// the reply word cap, followups against the followup cap.
func (s *Service) draftFollowup(ctx context.Context, userID uuid.UUID, in prompts.OutreachReplyInput) string {
	once := func() (*prompts.OutreachReplyResult, error) {
		p := prompts.BuildOutreachReplyPrompt(in)
		raw, _, err := s.ai.CompleteJSON(ai.WithUserID(ctx, userID), "outreach_reply", p.Prompt, p.System, 0)
		if err != nil {
			return nil, err
		}
		var out prompts.OutreachReplyResult
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	first, err := once()
	if err != nil || first == nil {
		return "(followup draft failed in simulation)"
	}
	mode := prompts.DraftModeFollowup
	if in.Mode == "reply" {
		mode = prompts.DraftModeReply
	}
	if len(prompts.ValidateOutreachBody(first.Body, in.HasCatalog, mode)) > 0 {
		in.StricterRetry = true
		if second, err := once(); err == nil && second != nil {
			return second.Body
		}
	}
	return first.Body
}

// classify runs the real sentiment classifier on a buyer reply and returns the
// legacy 3-value sentiment (for the branch), the 5-level label (for display,
// matching production), the score, and validated intent tags — same vocabulary
// + confidence floor + thresholds as the poller, so the Lab exercises the exact
// production path.
func (s *Service) classify(ctx context.Context, userID uuid.UUID, subject, body string) (legacy, level string, score float64, tags []string) {
	// Mirror the poller exactly: empty body short-circuits to neutral, the
	// score is clamped, and a missing score falls back to the legacy string.
	if strings.TrimSpace(body) == "" {
		return domain.SentimentNeutral, domain.SentimentNeutral, 0, nil
	}
	p := prompts.BuildOutreachSentimentPrompt(prompts.OutreachSentimentInput{Subject: subject, Body: body})
	raw, _, err := s.ai.CompleteJSON(ai.WithUserID(ctx, userID), "outreach_sentiment", p.Prompt, p.System, 0)
	if err != nil {
		return domain.SentimentNeutral, domain.SentimentNeutral, 0, nil
	}
	var out prompts.OutreachSentimentResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return domain.SentimentNeutral, domain.SentimentNeutral, 0, nil
	}
	if out.Score != nil {
		score = clampScore(*out.Score)
	} else {
		score = domain.ScoreForLegacy(out.Sentiment)
	}
	level = domain.LabelForScore(score)
	legacy = domain.LegacyForScore(score)
	seen := map[string]bool{}
	for _, t := range out.IntentTags {
		if !domain.IsValidIntentTag(t.Tag) || t.Confidence < 0.5 || seen[t.Tag] {
			continue
		}
		seen[t.Tag] = true
		tags = append(tags, t.Tag)
	}
	return legacy, level, score, tags
}

// clampScore mirrors the poller's clamp so Lab scores match production.
func clampScore(s float64) float64 {
	switch {
	case s < -1:
		return -1
	case s > 1:
		return 1
	default:
		return s
	}
}

// personaReply asks the buyer-bot to reply in character. Returns (body, intent).
func (s *Service) personaReply(ctx context.Context, userID uuid.UUID, persona Persona, bizContext string, transcript []TranscriptMessage) (string, string) {
	p := prompts.BuildPersonaReplyPrompt(prompts.PersonaReplyInput{
		PersonaPrompt:    persona.IntentPrompt,
		BusinessContext:  bizContext,
		ConversationText: renderThread(transcript),
	})
	raw, _, err := s.ai.CompleteJSON(ai.WithUserID(ctx, userID), "simulation_buyer_persona", p.Prompt, p.System, 0)
	if err != nil {
		return "(no reply)", ""
	}
	var out prompts.PersonaReplyResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return "(no reply)", ""
	}
	return out.Body, out.Intent
}

// judge grades a finished transcript. Returns raw JSON (JudgeResult shape).
func (s *Service) judge(ctx context.Context, userID uuid.UUID, persona Persona, outcome string, transcript []TranscriptMessage) json.RawMessage {
	p := prompts.BuildJudgePrompt(prompts.JudgeInput{
		PersonaLabel:     persona.Label,
		PersonaPrompt:    persona.IntentPrompt,
		Outcome:          outcome,
		ConversationText: renderThread(transcript),
	})
	raw, _, err := s.ai.CompleteJSON(ai.WithUserID(ctx, userID), "simulation_judge", p.Prompt, p.System, 0)
	if err != nil {
		return nil
	}
	var out prompts.JudgeResult
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return raw
}
