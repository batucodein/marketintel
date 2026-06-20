package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

// agent_tools.go holds the read-only tools the simulation agent calls during its
// ReAct loop. They use the SAME tool names as the group agent's shared prompt
// (counts, list_contacts, list_drafts, get_draft, list_replies, get_reply,
// get_lead, top_leads, get_brand, get_playbook, get_campaign_setup) but read the
// in-memory simulation state — so one prompt drives both agents.

const simSnippetMax = 220
const simBodyMax = 1500

// dispatchSimTool runs one read-only tool against the loaded simulation.
func (s *Service) dispatchSimTool(ctx context.Context, userID uuid.UUID, sim *Simulation, brand *domain.SenderProfile, name string, args json.RawMessage) string {
	switch name {
	case "counts":
		return simCounts(sim)
	case "list_contacts", "list_leads":
		return simListLeads(sim)
	case "list_drafts":
		return simListDrafts(sim, args)
	case "get_draft":
		return simGetDraft(sim, args)
	case "list_replies":
		return simListReplies(sim, args)
	case "get_reply":
		return simGetReply(sim, args)
	case "get_lead":
		return s.simGetLead(ctx, userID, sim, args)
	case "top_leads":
		return s.simTopLeads(ctx, userID, sim, args)
	case "get_brand":
		return simBrand(brand)
	case "get_playbook":
		return simPlaybookText(sim)
	case "get_campaign_setup":
		return simSetup(sim)
	default:
		return fmt.Sprintf("error: unknown tool %q.", name)
	}
}

func simCounts(sim *Simulation) string {
	var coldP, followP, replyP, awaiting, active, snoozed, done int
	var pos, neg, neu int
	outcomes := map[string]int{}
	for _, l := range sim.Leads {
		switch l.State {
		case stateAwaiting:
			awaiting++
			if l.PendingDraft != nil {
				switch l.PendingDraft.Kind {
				case "cold":
					coldP++
				case "followup":
					followP++
				case "reply":
					replyP++
				}
			}
		case stateActive:
			active++
		case stateSnoozed:
			snoozed++
		case stateDone:
			done++
			outcomes[l.Outcome]++
		}
		if l.LastSentiment != nil {
			switch *l.LastSentiment {
			case domain.SentimentPositive:
				pos++
			case domain.SentimentNegative:
				neg++
			case domain.SentimentNeutral:
				neu++
			}
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Virtual day: %d\n", sim.VirtualDay)
	fmt.Fprintf(&b, "Drafts awaiting approval: %d (cold %d, follow-up %d, reply %d)\n", awaiting, coldP, followP, replyP)
	fmt.Fprintf(&b, "Leads — active(in cadence): %d, snoozed: %d, done: %d\n", active, snoozed, done)
	fmt.Fprintf(&b, "Replied so far — positive: %d, negative: %d, neutral: %d\n", pos, neg, neu)
	if len(outcomes) > 0 {
		var parts []string
		for k, v := range outcomes {
			parts = append(parts, fmt.Sprintf("%s: %d", k, v))
		}
		sort.Strings(parts)
		fmt.Fprintf(&b, "Final outcomes — %s", strings.Join(parts, ", "))
	}
	return strings.TrimSpace(b.String())
}

func simStateLabel(state string) string {
	switch state {
	case stateAwaiting:
		return "awaiting approval"
	case stateActive:
		return "in cadence"
	case stateSnoozed:
		return "snoozed"
	default:
		return "done"
	}
}

func simListLeads(sim *Simulation) string {
	if len(sim.Leads) == 0 {
		return "This simulation has no leads."
	}
	var b strings.Builder
	for _, l := range sim.Leads {
		line := fmt.Sprintf("- %s (%s) — %s", l.DisplayName, strings.ReplaceAll(l.Persona, "_", " "), simStateLabel(l.State))
		if l.State == stateDone && l.Outcome != "" {
			line += " · " + strings.ReplaceAll(l.Outcome, "_", " ")
		}
		b.WriteString(line + "\n")
	}
	return fmt.Sprintf("%d lead(s):\n%s", len(sim.Leads), strings.TrimSpace(b.String()))
}

func simListDrafts(sim *Simulation, args json.RawMessage) string {
	var a struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(args, &a)
	var b strings.Builder
	n := 0
	for _, l := range sim.Leads {
		if l.State != stateAwaiting || l.PendingDraft == nil {
			continue
		}
		pd := l.PendingDraft
		if a.Kind != "" && a.Kind != pd.Kind {
			continue
		}
		fmt.Fprintf(&b, "- %s | %s | %s | %s\n", l.DisplayName, pd.Kind, pd.Subject, truncate(pd.Body, simSnippetMax))
		n++
	}
	if n == 0 {
		return "No drafts awaiting approval."
	}
	return fmt.Sprintf("%d draft(s) awaiting approval:\n%s", n, strings.TrimSpace(b.String()))
}

func simGetDraft(sim *Simulation, args json.RawMessage) string {
	var a struct {
		Contact string `json:"contact"`
	}
	_ = json.Unmarshal(args, &a)
	l, errStr := resolveSimLead(sim, a.Contact)
	if errStr != "" {
		return errStr
	}
	if l.PendingDraft == nil {
		return fmt.Sprintf("error: %s has no draft awaiting approval.", l.DisplayName)
	}
	pd := l.PendingDraft
	return fmt.Sprintf("Draft for %s (%s)\nSubject: %s\n\n%s", l.DisplayName, pd.Kind, pd.Subject, truncate(pd.Body, simBodyMax))
}

func simListReplies(sim *Simulation, args json.RawMessage) string {
	var a struct {
		Sentiment string `json:"sentiment"`
		Tag       string `json:"tag"`
	}
	_ = json.Unmarshal(args, &a)
	var b strings.Builder
	n := 0
	for _, l := range sim.Leads {
		last := lastInbound(l)
		if last == nil {
			continue
		}
		if a.Sentiment != "" && last.Sentiment != a.Sentiment {
			continue
		}
		if a.Tag != "" && !hasTag(last.Tags, a.Tag) {
			continue
		}
		line := fmt.Sprintf("- %s — %s", l.DisplayName, last.Sentiment)
		if len(last.Tags) > 0 {
			line += " [" + strings.Join(last.Tags, ", ") + "]"
		}
		b.WriteString(line + "\n")
		n++
	}
	if n == 0 {
		return "No replies match that filter."
	}
	return fmt.Sprintf("%d replier(s):\n%s", n, strings.TrimSpace(b.String()))
}

func simGetReply(sim *Simulation, args json.RawMessage) string {
	var a struct {
		Contact string `json:"contact"`
	}
	_ = json.Unmarshal(args, &a)
	l, errStr := resolveSimLead(sim, a.Contact)
	if errStr != "" {
		return errStr
	}
	last := lastInbound(*l)
	if last == nil {
		return fmt.Sprintf("error: %s hasn't replied.", l.DisplayName)
	}
	tagStr := "(none)"
	if len(last.Tags) > 0 {
		tagStr = strings.Join(last.Tags, ", ")
	}
	return fmt.Sprintf("Latest reply from %s\nSentiment: %s | Tags: %s\n\n%s", l.DisplayName, last.Sentiment, tagStr, truncate(last.Body, simBodyMax))
}

func (s *Service) simGetLead(ctx context.Context, userID uuid.UUID, sim *Simulation, args json.RawMessage) string {
	var a struct {
		Contact string `json:"contact"`
	}
	_ = json.Unmarshal(args, &a)
	l, errStr := resolveSimLead(sim, a.Contact)
	if errStr != "" {
		return errStr
	}
	if l.BusinessID == nil {
		return fmt.Sprintf("error: %s has no business record.", l.DisplayName)
	}
	score := s.leadCtx.LeadScoreJSON(ctx, userID, *l.BusinessID)
	trade := s.leadCtx.ShipmentContext(ctx, *l.BusinessID)
	business := s.leadCtx.BusinessJSON(ctx, *l.BusinessID)
	if score == "" {
		score = "(not scored)"
	}
	if trade == "" {
		trade = "(no shipment data)"
	}
	return fmt.Sprintf("Lead: %s (%s)\nLead score: %s\nTrade: %s\nBusiness: %s",
		l.DisplayName, strings.ReplaceAll(l.Persona, "_", " "), score, trade, truncate(business, simBodyMax))
}

func (s *Service) simTopLeads(ctx context.Context, userID uuid.UUID, sim *Simulation, args json.RawMessage) string {
	// Without a SQL ranking surface here, list each lead's overall score from the
	// loader so the agent can compare (small N).
	type row struct {
		name  string
		score int
		has   bool
	}
	var rows []row
	for _, l := range sim.Leads {
		if l.BusinessID == nil {
			continue
		}
		var sc struct {
			Overall *int `json:"overall_score"`
		}
		_ = json.Unmarshal([]byte(s.leadCtx.LeadScoreJSON(ctx, userID, *l.BusinessID)), &sc)
		r := row{name: l.DisplayName}
		if sc.Overall != nil {
			r.score, r.has = *sc.Overall, true
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].score > rows[j].score })
	var b strings.Builder
	n := 0
	for _, r := range rows {
		if !r.has {
			continue
		}
		fmt.Fprintf(&b, "- %s (%d)\n", r.name, r.score)
		n++
	}
	if n == 0 {
		return "No scored leads in this simulation."
	}
	return "Leads by overall score:\n" + strings.TrimSpace(b.String())
}

func simBrand(brand *domain.SenderProfile) string {
	if brand == nil {
		return "error: no brand."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Brand: %s\n", brand.CompanyName)
	add := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, truncate(val, 400))
		}
	}
	add("Product", brand.ProductDescription)
	add("Value proposition", brand.ValueProp)
	add("Target buyer", brand.TargetBuyerDescription)
	add("Tone", brand.Tone)
	add("Deal-breakers", brand.DealBreakers)
	add("Competitive moats", brand.CompetitiveMoats)
	return strings.TrimSpace(b.String())
}

func simPlaybookText(sim *Simulation) string {
	if len(sim.Playbook) == 0 {
		return "(playbook is empty)"
	}
	keys := make([]string, 0, len(sim.Playbook))
	for k := range sim.Playbook {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "- when reply is %s: %s\n", k, sim.Playbook[k])
	}
	return strings.TrimSpace(b.String())
}

func simSetup(sim *Simulation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "On positive reply: %s\n", d(sim.OnPositiveAction, "(default)"))
	fmt.Fprintf(&b, "On negative reply: %s\n", d(sim.OnNegativeAction, "(default)"))
	if len(sim.Steps) == 0 {
		b.WriteString("Follow-up cadence: none")
		return strings.TrimSpace(b.String())
	}
	b.WriteString("Follow-up cadence:\n")
	for _, st := range sim.Steps {
		fmt.Fprintf(&b, "- step %d: wait %dd, on %s → %s\n", st.StepNumber, st.WaitDays, st.Trigger, st.Action)
	}
	return strings.TrimSpace(b.String())
}

// resolveSimLead finds a lead by display-name substring; returns an error string
// for no/ambiguous match (the model recovers via list_contacts).
func resolveSimLead(sim *Simulation, name string) (*Lead, string) {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return nil, "error: provide a contact name."
	}
	var matches []*Lead
	for i := range sim.Leads {
		if strings.Contains(strings.ToLower(sim.Leads[i].DisplayName), name) {
			matches = append(matches, &sim.Leads[i])
		}
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Sprintf("error: no lead matching %q. Use list_contacts to see valid names.", name)
	case 1:
		return matches[0], ""
	default:
		var names []string
		for _, m := range matches {
			names = append(names, m.DisplayName)
		}
		return nil, fmt.Sprintf("error: %q is ambiguous — matches: %s.", name, strings.Join(names, ", "))
	}
}

// lastInbound returns a lead's most recent persona reply, or nil.
func lastInbound(l Lead) *TranscriptMessage {
	for i := len(l.Transcript) - 1; i >= 0; i-- {
		if l.Transcript[i].Who == "them" {
			return &l.Transcript[i]
		}
	}
	return nil
}

// truncate trims s to n runes with an ellipsis.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// d returns s, or fallback when s is blank.
func d(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
