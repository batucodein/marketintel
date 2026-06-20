package sequence

import (
	"time"

	"github.com/batuhan/marketintel/internal/domain"
)

// EvaluateTriggerPure is the pure decision behind a sequence step's trigger.
// It has no DB or AI dependencies so the same logic can be exercised by the
// real engine (which loads the conversation + classifies sentiment first) and
// by the simulation harness (which supplies an in-memory snapshot).
//
//   - trigger:             the step trigger (no_reply / any_reply / positive_reply / always)
//   - waitDays:            the step's wait period
//   - lastDirection:       conversation's last message direction ("out"/"in"), nil if none
//   - lastMessageAt:       when that last message happened, zero if none
//   - now:                 the reference clock (real time, or the sim's virtual time)
//   - lastInboundPositive: for positive_reply, whether the last inbound was classified
//     positive (nil = not an inbound / not classified)
func EvaluateTriggerPure(
	trigger string,
	waitDays int,
	lastDirection *string,
	lastMessageAt time.Time,
	now time.Time,
	lastInboundPositive *bool,
) bool {
	switch trigger {
	case domain.SequenceTriggerAlways:
		return true
	case domain.SequenceTriggerNoReply:
		// Fires only if the last message was outbound (no reply yet) AND
		// wait_days has elapsed since it.
		if lastDirection == nil || *lastDirection != domain.DirectionOutbound {
			return false
		}
		if lastMessageAt.IsZero() {
			return false
		}
		return now.Sub(lastMessageAt) >= time.Duration(waitDays)*24*time.Hour
	case domain.SequenceTriggerAnyReply:
		return lastDirection != nil && *lastDirection == domain.DirectionInbound
	case domain.SequenceTriggerPositiveReply:
		if lastDirection == nil || *lastDirection != domain.DirectionInbound {
			return false
		}
		return lastInboundPositive != nil && *lastInboundPositive
	default:
		return false
	}
}

// Reply routing decisions. An intent tag on the inbound reply can override the
// sentiment branch.
const (
	RouteSentiment = "sentiment_branch" // no overriding intent → use positive/negative branch
	RouteSuppress  = "suppress"         // unsubscribe → suppress contact (compliance)
	RouteSnooze    = "no_draft_snooze"  // out_of_office → don't draft; wait & resume
	RouteNotify    = "notify"           // wrong_contact → surface a task, no draft
)

// ResolveIntentRouting decides whether an intent tag overrides the sentiment
// branch. Precedence: unsubscribe (compliance) > out_of_office > wrong_contact >
// none. Pure + unit-tested; mirrored by the simulation so the Lab stays honest.
func ResolveIntentRouting(tags []string) string {
	has := func(t string) bool {
		for _, x := range tags {
			if x == t {
				return true
			}
		}
		return false
	}
	switch {
	case has(domain.TagUnsubscribe):
		return RouteSuppress
	case has(domain.TagOutOfOffice):
		return RouteSnooze
	case has(domain.TagWrongContact):
		return RouteNotify
	default:
		return RouteSentiment
	}
}
