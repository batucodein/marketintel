package simulation

import (
	"testing"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/sequence"
)

func TestDeriveOutcome(t *testing.T) {
	cases := []struct {
		unsub, cold, replied bool
		sentiment            string
		want                 string
	}{
		{true, false, true, "negative", "unsubscribed"},
		{false, true, false, "", "cold"},
		{false, false, true, "positive", "replied_positive"},
		{false, false, true, "negative", "replied_negative"},
		{false, false, true, "neutral", "replied"},
		{false, false, false, "", "no_reply"},
	}
	for _, c := range cases {
		if got := deriveOutcome(c.unsub, c.cold, c.replied, c.sentiment); got != c.want {
			t.Errorf("deriveOutcome(%v,%v,%v,%q)=%q want %q", c.unsub, c.cold, c.replied, c.sentiment, got, c.want)
		}
	}
}

func TestSimScopeMatches(t *testing.T) {
	pos, neg := domain.SentimentPositive, domain.SentimentNegative
	reply := func(sent *string) *Lead { return &Lead{PendingDraft: &PendingDraft{Kind: "reply"}, LastSentiment: sent} }
	cold := &Lead{PendingDraft: &PendingDraft{Kind: "cold"}}

	if !simScopeMatches("all", cold) {
		t.Error("all should match cold")
	}
	if !simScopeMatches("cold", cold) || simScopeMatches("reply", cold) {
		t.Error("cold scope mismatch")
	}
	if !simScopeMatches("reply_positive", reply(&pos)) {
		t.Error("reply_positive should match positive reply")
	}
	if simScopeMatches("reply_positive", reply(&neg)) {
		t.Error("reply_positive should NOT match negative reply")
	}
	if !simScopeMatches("reply", reply(&neg)) {
		t.Error("reply scope should match any reply")
	}
}

// TestCadenceDayCursor checks the virtual-day accumulation matches runLead's
// cumulative dayCursor: step i is due at sum(wait_days[0..i]).
func TestCadenceDayCursor(t *testing.T) {
	s := &Service{}
	steps := []domain.SequenceStep{
		{StepNumber: 1, WaitDays: 0, Trigger: domain.SequenceTriggerNoReply, Action: domain.SequenceActionSendMessage},
		{StepNumber: 2, WaitDays: 3, Trigger: domain.SequenceTriggerNoReply, Action: domain.SequenceActionSendMessage},
		{StepNumber: 3, WaitDays: 5, Trigger: domain.SequenceTriggerNoReply, Action: domain.SequenceActionMarkCold},
	}
	lead := &Lead{LastOutDay: 0, CurrentStep: 0, NextDay: 0}

	// After the cold send, step 0 is scheduled relative to it.
	s.scheduleCold(lead, steps)
	if lead.NextDay != 0 {
		t.Fatalf("step0 due day = %d, want 0", lead.NextDay)
	}
	// Step 0 fires (no reply) → advance to step 1: cumulative 0+3.
	s.advanceCursor(lead, steps)
	if lead.CurrentStep != 1 || lead.NextDay != 3 {
		t.Fatalf("after step0: step=%d nextDay=%d, want step=1 nextDay=3", lead.CurrentStep, lead.NextDay)
	}
	// Step 1 fires → advance to step 2: cumulative 3+5.
	s.advanceCursor(lead, steps)
	if lead.CurrentStep != 2 || lead.NextDay != 8 {
		t.Fatalf("after step1: step=%d nextDay=%d, want step=2 nextDay=8", lead.CurrentStep, lead.NextDay)
	}
}

// TestTriggerAgreesWithDayClock confirms the virtual day clock + EvaluateTriggerPure
// fire a no_reply step exactly when wait_days have elapsed since the last outbound.
func TestTriggerAgreesWithDayClock(t *testing.T) {
	out := domain.DirectionOutbound
	lastOutDay := 2
	waitDays := 3
	// Not yet due at day 4 (only 2 elapsed).
	if sequence.EvaluateTriggerPure(domain.SequenceTriggerNoReply, waitDays, &out, dayToTime(lastOutDay), dayToTime(4), nil) {
		t.Error("trigger fired too early (2 < 3 days elapsed)")
	}
	// Due at day 5 (3 elapsed).
	if !sequence.EvaluateTriggerPure(domain.SequenceTriggerNoReply, waitDays, &out, dayToTime(lastOutDay), dayToTime(5), nil) {
		t.Error("trigger should fire at exactly wait_days elapsed")
	}
}
