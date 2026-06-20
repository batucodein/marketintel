package sequence

import (
	"testing"
	"time"

	"github.com/batuhan/marketintel/internal/domain"
)

func strptr(s string) *string { return &s }
func boolptr(b bool) *bool    { return &b }

func TestEvaluateTriggerPure(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	out := strptr(domain.DirectionOutbound)
	in := strptr(domain.DirectionInbound)
	threeDaysAgo := now.Add(-3 * 24 * time.Hour)
	oneDayAgo := now.Add(-1 * 24 * time.Hour)

	cases := []struct {
		name     string
		trigger  string
		waitDays int
		lastDir  *string
		lastAt   time.Time
		positive *bool
		want     bool
	}{
		{"always fires", domain.SequenceTriggerAlways, 3, nil, time.Time{}, nil, true},
		{"no_reply: outbound + waited → fire", domain.SequenceTriggerNoReply, 2, out, threeDaysAgo, nil, true},
		{"no_reply: outbound but not waited → no", domain.SequenceTriggerNoReply, 3, out, oneDayAgo, nil, false},
		{"no_reply: they replied (inbound) → no (stops bumps)", domain.SequenceTriggerNoReply, 0, in, oneDayAgo, nil, false},
		{"no_reply: no last message → no", domain.SequenceTriggerNoReply, 0, out, time.Time{}, nil, false},
		{"any_reply: inbound → fire", domain.SequenceTriggerAnyReply, 0, in, oneDayAgo, nil, true},
		{"any_reply: outbound → no", domain.SequenceTriggerAnyReply, 0, out, oneDayAgo, nil, false},
		{"positive_reply: inbound + positive → fire", domain.SequenceTriggerPositiveReply, 0, in, oneDayAgo, boolptr(true), true},
		{"positive_reply: inbound + negative → no", domain.SequenceTriggerPositiveReply, 0, in, oneDayAgo, boolptr(false), false},
		{"positive_reply: inbound + unclassified → no", domain.SequenceTriggerPositiveReply, 0, in, oneDayAgo, nil, false},
		{"positive_reply: outbound → no", domain.SequenceTriggerPositiveReply, 0, out, oneDayAgo, boolptr(true), false},
		{"unknown trigger → no", "bogus", 0, in, oneDayAgo, boolptr(true), false},
	}

	for _, c := range cases {
		got := EvaluateTriggerPure(c.trigger, c.waitDays, c.lastDir, c.lastAt, now, c.positive)
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

func TestResolveReplyBranch(t *testing.T) {
	cases := []struct {
		sentiment, onPos, onNeg string
		wantBranch, wantAction  string
	}{
		{domain.SentimentPositive, domain.OnPositiveAutoDraftReply, domain.OnNegativeMarkCold, "positive", domain.OnPositiveAutoDraftReply},
		{domain.SentimentNeutral, domain.OnPositiveAutoDraftReply, domain.OnNegativeMarkCold, "positive", domain.OnPositiveAutoDraftReply},
		{domain.SentimentNeutral, domain.OnPositiveNotify, domain.OnNegativeMarkCold, "positive", domain.OnPositiveNotify},
		{domain.SentimentNegative, domain.OnPositiveAutoDraftReply, domain.OnNegativeMarkCold, "negative", domain.OnNegativeMarkCold},
		{domain.SentimentNegative, domain.OnPositiveAutoDraftReply, domain.OnNegativeNotify, "negative", domain.OnNegativeNotify},
	}
	for _, c := range cases {
		b, a := ResolveReplyBranch(c.sentiment, c.onPos, c.onNeg)
		if b != c.wantBranch || a != c.wantAction {
			t.Errorf("sentiment=%s: got (%s,%s), want (%s,%s)", c.sentiment, b, a, c.wantBranch, c.wantAction)
		}
	}
}

func TestResolveIntentRouting(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"no tags → sentiment branch", nil, RouteSentiment},
		{"only buying tags → sentiment branch", []string{domain.TagPriceRequested, domain.TagMeetingRequested}, RouteSentiment},
		{"unsubscribe → suppress", []string{domain.TagUnsubscribe}, RouteSuppress},
		{"out_of_office → snooze", []string{domain.TagOutOfOffice}, RouteSnooze},
		{"wrong_contact → notify", []string{domain.TagWrongContact}, RouteNotify},
		{"unsubscribe beats OOO", []string{domain.TagOutOfOffice, domain.TagUnsubscribe}, RouteSuppress},
		{"OOO beats wrong_contact", []string{domain.TagWrongContact, domain.TagOutOfOffice}, RouteSnooze},
		{"wrong_contact beats a buying tag", []string{domain.TagPriceRequested, domain.TagWrongContact}, RouteNotify},
	}
	for _, c := range cases {
		if got := ResolveIntentRouting(c.tags); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}
