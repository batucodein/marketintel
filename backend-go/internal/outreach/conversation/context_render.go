package conversation

import (
	"fmt"
	"strings"
	"time"

	"github.com/batuhan/marketintel/internal/domain"
)

// RenderPriorConversations turns a slice of ThreadWithMessages into the
// compact text block that goes into the AI draft prompt. The output is
// "human-readable summary, oldest message first within each thread,
// threads newest first" — designed to be scannable by the LLM without
// burning a huge context budget.
//
// Returns "" when there is nothing useful to show (no threads, or every
// thread is empty). The caller passes "" through as "no prior context".
func RenderPriorConversations(threads []ThreadWithMessages, sinceFmt time.Time) string {
	if len(threads) == 0 {
		return ""
	}
	var b strings.Builder
	rendered := 0
	for _, t := range threads {
		if len(t.Messages) == 0 {
			continue
		}
		// Header for the thread
		subject := "(no subject)"
		if t.Conversation.Subject != nil && *t.Conversation.Subject != "" {
			subject = *t.Conversation.Subject
		}
		when := t.Conversation.CreatedAt
		if t.Conversation.LastMessageAt != nil {
			when = *t.Conversation.LastMessageAt
		}
		b.WriteString(fmt.Sprintf("--- Thread: %q (last activity %s, %d messages) ---\n",
			subject, when.Format("2006-01-02"), len(t.Messages)))

		for _, m := range t.Messages {
			who := "YOU"
			if m.Direction == domain.DirectionInbound {
				who = "THEM"
			}
			body := ""
			if m.BodyText != nil {
				body = *m.BodyText
			} else if m.BodyHTML != nil {
				body = *m.BodyHTML
			}
			// Squeeze excessive whitespace; cap each message at ~600
			// chars so a single huge thread doesn't dominate the prompt.
			body = compactWhitespace(body)
			if len(body) > 600 {
				body = body[:597] + "..."
			}
			ts := ""
			if m.SentAt != nil {
				ts = m.SentAt.Format("2006-01-02")
			} else if m.ReceivedAt != nil {
				ts = m.ReceivedAt.Format("2006-01-02")
			}
			b.WriteString(fmt.Sprintf("[%s · %s] %s\n", who, ts, body))
		}
		b.WriteString("\n")
		rendered++
	}
	if rendered == 0 {
		return ""
	}
	return strings.TrimSpace(b.String())
}

func compactWhitespace(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}
