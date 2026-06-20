package campaign

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// agent_tools.go holds the read-only tools the group draft agent calls during
// its ReAct loop (see assistant.go::AssistantSend). Every tool is scoped to the
// (userID, campaignID) the loop captured — the model never passes ids, so it
// can only ever read THIS group's data. Each tool returns a plain string; on a
// bad/empty input it returns a short "error: …" the model can recover from by
// calling a discovery tool (list_replies / list_drafts) and retrying.

const (
	agentListLimit    = 25 // default rows for list_* tools
	agentListLimitMax = 50
	agentSnippetMax   = 220
	agentBodyMax      = 1500
)

var agentReplySentiments = map[string]bool{"positive": true, "negative": true, "neutral": true}

// leadScoreColumns whitelists the sortable lead-score dimensions for top_leads
// (maps the agent's dimension arg → the real column; prevents SQL injection
// since the value is interpolated into ORDER BY).
var leadScoreColumns = map[string]string{
	"overall":              "overall_score",
	"overall_score":        "overall_score",
	"purchase_likelihood":  "purchase_likelihood",
	"deal_size_potential":  "deal_size_potential",
	"urgency":              "urgency_score",
	"urgency_score":        "urgency_score",
	"fit":                  "fit_score",
	"fit_score":            "fit_score",
	"accessibility":        "accessibility_score",
	"accessibility_score":  "accessibility_score",
}

// dispatchAgentTool runs one read-only tool and returns its result rendered as
// text for the next loop step. camp is the (already ownership-checked) campaign.
func (s *Service) dispatchAgentTool(ctx context.Context, userID uuid.UUID, camp *domain.Campaign, call *prompts.AgentToolCall) string {
	switch call.Name {
	case "counts":
		return s.renderCounts(ctx, camp.ID)
	case "list_contacts":
		return s.toolListContacts(ctx, camp.ID, call.Args)
	case "list_drafts":
		return s.toolListDrafts(ctx, camp.ID, call.Args)
	case "get_draft":
		return s.toolGetDraft(ctx, camp.ID, call.Args)
	case "list_replies":
		return s.toolListReplies(ctx, camp.ID, call.Args)
	case "get_reply":
		return s.toolGetReply(ctx, camp.ID, call.Args)
	case "get_lead":
		return s.toolGetLead(ctx, userID, camp.ID, call.Args)
	case "top_leads":
		return s.toolTopLeads(ctx, userID, camp.ID, call.Args)
	case "get_brand":
		return s.toolGetBrand(ctx, camp)
	case "get_playbook":
		if pb := s.renderPlaybook(ctx, camp.ID); pb != "" {
			return pb
		}
		return "(playbook is empty)"
	case "get_campaign_setup":
		return s.toolCampaignSetup(ctx, camp)
	default:
		return fmt.Sprintf("error: unknown tool %q. Valid tools: counts, list_drafts, get_draft, list_replies, get_reply, get_lead, top_leads, get_brand, get_playbook, get_campaign_setup.", call.Name)
	}
}

// agentContact is a campaign member resolved from a free-text name.
type agentContact struct {
	contactID      uuid.UUID
	name           string
	businessID     *uuid.UUID
	conversationID *uuid.UUID
	draftMessageID *uuid.UUID
}

// resolveAgentContact finds campaign members whose display name matches (ILIKE
// substring), capped so the dispatcher can detect ambiguity.
func (s *Service) resolveAgentContact(ctx context.Context, campaignID uuid.UUID, name string) ([]agentContact, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx,
		`SELECT cc.contact_id, ct.display_name, ct.business_id, cc.conversation_id, cc.draft_message_id
		   FROM campaign_contacts cc
		   JOIN contacts ct ON ct.id = cc.contact_id
		  WHERE cc.campaign_id = $1 AND ct.display_name ILIKE '%' || $2 || '%'
		  ORDER BY ct.display_name LIMIT 6`, campaignID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []agentContact
	for rows.Next() {
		var c agentContact
		if err := rows.Scan(&c.contactID, &c.name, &c.businessID, &c.conversationID, &c.draftMessageID); err != nil {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// oneContact resolves a name to exactly one member, or returns an error string
// (no match / ambiguous) for the model to recover from.
func (s *Service) oneContact(ctx context.Context, campaignID uuid.UUID, name string) (*agentContact, string) {
	matches, err := s.resolveAgentContact(ctx, campaignID, name)
	if err != nil {
		return nil, "error: lookup failed."
	}
	switch len(matches) {
	case 0:
		return nil, fmt.Sprintf("error: no contact in this group matching %q. Use list_replies or list_drafts to see valid names.", name)
	case 1:
		return &matches[0], ""
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, m.name)
		}
		return nil, fmt.Sprintf("error: %q is ambiguous — matches: %s. Use a more specific name.", name, strings.Join(names, ", "))
	}
}

func agentLimit(raw int) int {
	if raw <= 0 {
		return agentListLimit
	}
	if raw > agentListLimitMax {
		return agentListLimitMax
	}
	return raw
}

// toolListContacts lists the group's members (companies) regardless of draft or
// reply state — the entry point for "who/what companies are in this group".
func (s *Service) toolListContacts(ctx context.Context, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(args, &a)
	limit := agentLimit(a.Limit)
	rows, err := s.pool.Query(ctx,
		`SELECT ct.display_name, COALESCE(b.name,''), COALESCE(b.country_code,''), cc.status
		   FROM campaign_contacts cc
		   JOIN contacts ct ON ct.id = cc.contact_id
		   LEFT JOIN businesses b ON b.id = ct.business_id
		  WHERE cc.campaign_id=$1
		  ORDER BY ct.display_name LIMIT $2`, campaignID, limit)
	if err != nil {
		return "error: could not list contacts."
	}
	defer rows.Close()
	var b strings.Builder
	n := 0
	for rows.Next() {
		var name, biz, country, status string
		if rows.Scan(&name, &biz, &country, &status) != nil {
			continue
		}
		who := name
		if biz != "" {
			who = fmt.Sprintf("%s [%s%s]", name, biz, func() string {
				if country != "" {
					return ", " + country
				}
				return ""
			}())
		}
		fmt.Fprintf(&b, "- %s — %s\n", who, status)
		n++
	}
	if n == 0 {
		return "This group has no contacts yet."
	}
	var total int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM campaign_contacts WHERE campaign_id=$1`, campaignID).Scan(&total)
	header := fmt.Sprintf("%d contact(s)", n)
	if total > n {
		header = fmt.Sprintf("showing %d of %d contact(s)", n, total)
	}
	return header + ":\n" + strings.TrimSpace(b.String()) + "\n(Use get_lead with a name for full company detail, or top_leads to rank by score.)"
}

func (s *Service) toolListDrafts(ctx context.Context, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Kind      string `json:"kind"`
		Sentiment string `json:"sentiment"`
		Limit     int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &a)
	limit := agentLimit(a.Limit)
	switch a.Kind {
	case "cold":
		rows, err := s.pool.Query(ctx,
			`SELECT ct.display_name, COALESCE(m.subject,''), COALESCE(m.body_text,'')
			   FROM campaign_contacts cc
			   JOIN contacts ct ON ct.id = cc.contact_id
			   JOIN messages m ON m.id = cc.draft_message_id
			  WHERE cc.campaign_id=$1 AND cc.status='drafted' AND cc.draft_message_id IS NOT NULL
			  ORDER BY ct.display_name LIMIT $2`, campaignID, limit)
		if err != nil {
			return "error: could not list cold drafts."
		}
		return renderDraftList(rows, "cold opener drafts awaiting approval")
	case "reply":
		q := `SELECT ct.display_name, COALESCE(m.subject,''), COALESCE(m.body_text,'')
		        FROM conversations cv
		        JOIN messages m ON m.conversation_id = cv.id AND m.direction='out' AND m.status='pending_approval'
		        JOIN contacts ct ON ct.id = cv.contact_id
		       WHERE cv.campaign_id=$1`
		cargs := []any{campaignID}
		if a.Sentiment != "" {
			if !agentReplySentiments[a.Sentiment] {
				return "error: sentiment must be positive, negative, or neutral."
			}
			q += ` AND cv.last_inbound_sentiment=$2`
			cargs = append(cargs, a.Sentiment)
		}
		q += fmt.Sprintf(` ORDER BY ct.display_name LIMIT %d`, limit)
		rows, err := s.pool.Query(ctx, q, cargs...)
		if err != nil {
			return "error: could not list reply drafts."
		}
		return renderDraftList(rows, "reply drafts awaiting approval")
	default:
		return `error: kind must be "cold" or "reply".`
	}
}

func (s *Service) toolGetDraft(ctx context.Context, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Kind    string `json:"kind"`
		Contact string `json:"contact"`
	}
	_ = json.Unmarshal(args, &a)
	c, errStr := s.oneContact(ctx, campaignID, a.Contact)
	if errStr != "" {
		return errStr
	}
	var subject, body string
	switch a.Kind {
	case "cold":
		if c.draftMessageID == nil {
			return fmt.Sprintf("error: %s has no cold draft awaiting approval.", c.name)
		}
		if err := s.pool.QueryRow(ctx,
			`SELECT COALESCE(subject,''), COALESCE(body_text,'') FROM messages WHERE id=$1`,
			*c.draftMessageID).Scan(&subject, &body); err != nil {
			return "error: could not load the draft."
		}
	case "reply":
		if c.conversationID == nil {
			return fmt.Sprintf("error: %s has no conversation, so no reply draft.", c.name)
		}
		if err := s.pool.QueryRow(ctx,
			`SELECT COALESCE(subject,''), COALESCE(body_text,'') FROM messages
			  WHERE conversation_id=$1 AND direction='out' AND status='pending_approval'
			  ORDER BY created_at DESC LIMIT 1`, *c.conversationID).Scan(&subject, &body); err != nil {
			return fmt.Sprintf("error: %s has no reply draft awaiting approval.", c.name)
		}
	default:
		return `error: kind must be "cold" or "reply".`
	}
	return fmt.Sprintf("Draft for %s\nSubject: %s\n\n%s", c.name, subject, truncate(body, agentBodyMax))
}

func (s *Service) toolListReplies(ctx context.Context, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Sentiment string `json:"sentiment"`
		Tag       string `json:"tag"`
	}
	_ = json.Unmarshal(args, &a)
	q := `SELECT ct.display_name, COALESCE(cv.last_inbound_sentiment,'?'),
	             COALESCE(ARRAY(SELECT tag FROM conversation_tags WHERE conversation_id=cv.id ORDER BY tag), '{}'),
	             EXISTS(SELECT 1 FROM messages pm WHERE pm.conversation_id=cv.id AND pm.direction='out' AND pm.status='pending_approval')
	        FROM campaign_contacts cc
	        JOIN contacts ct ON ct.id=cc.contact_id
	        JOIN conversations cv ON cv.id=cc.conversation_id
	       WHERE cc.campaign_id=$1 AND cv.last_direction='in'`
	cargs := []any{campaignID}
	if a.Sentiment != "" {
		if !agentReplySentiments[a.Sentiment] {
			return "error: sentiment must be positive, negative, or neutral."
		}
		cargs = append(cargs, a.Sentiment)
		q += fmt.Sprintf(` AND cv.last_inbound_sentiment=$%d`, len(cargs))
	}
	if a.Tag != "" {
		if !domain.IsValidIntentTag(a.Tag) {
			return fmt.Sprintf("error: %q is not a valid intent tag.", a.Tag)
		}
		cargs = append(cargs, a.Tag)
		q += fmt.Sprintf(` AND cv.id IN (SELECT conversation_id FROM conversation_tags WHERE tag=$%d)`, len(cargs))
	}
	q += ` ORDER BY ct.display_name LIMIT 60`
	rows, err := s.pool.Query(ctx, q, cargs...)
	if err != nil {
		return "error: could not list replies."
	}
	defer rows.Close()
	var b strings.Builder
	n := 0
	for rows.Next() {
		var name, sent string
		var tags []string
		var pending bool
		if rows.Scan(&name, &sent, &tags, &pending) != nil {
			continue
		}
		line := fmt.Sprintf("- %s — %s", name, sent)
		if len(tags) > 0 {
			line += " [" + strings.Join(tags, ", ") + "]"
		}
		if pending {
			line += " (reply draft awaiting approval)"
		}
		b.WriteString(line + "\n")
		n++
	}
	if n == 0 {
		return "No replies match that filter."
	}
	return fmt.Sprintf("%d replier(s):\n%s", n, strings.TrimSpace(b.String()))
}

func (s *Service) toolGetReply(ctx context.Context, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Contact string `json:"contact"`
	}
	_ = json.Unmarshal(args, &a)
	c, errStr := s.oneContact(ctx, campaignID, a.Contact)
	if errStr != "" {
		return errStr
	}
	if c.conversationID == nil {
		return fmt.Sprintf("error: %s has no conversation yet (hasn't replied).", c.name)
	}
	var body, sentiment string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(m.body_text,''), COALESCE(cv.last_inbound_sentiment,'?')
		   FROM messages m
		   JOIN conversations cv ON cv.id = m.conversation_id
		  WHERE m.conversation_id=$1 AND m.direction='in'
		  ORDER BY m.created_at DESC LIMIT 1`, *c.conversationID).Scan(&body, &sentiment); err != nil {
		return fmt.Sprintf("error: no inbound message found for %s.", c.name)
	}
	var tags []string
	_ = s.pool.QueryRow(ctx,
		`SELECT COALESCE(ARRAY(SELECT tag FROM conversation_tags WHERE conversation_id=$1 ORDER BY tag), '{}')`,
		*c.conversationID).Scan(&tags)
	tagStr := "(none)"
	if len(tags) > 0 {
		tagStr = strings.Join(tags, ", ")
	}
	return fmt.Sprintf("Latest reply from %s\nSentiment: %s | Tags: %s\n\n%s",
		c.name, sentiment, tagStr, truncate(body, agentBodyMax))
}

func (s *Service) toolGetLead(ctx context.Context, userID, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Contact string `json:"contact"`
	}
	_ = json.Unmarshal(args, &a)
	c, errStr := s.oneContact(ctx, campaignID, a.Contact)
	if errStr != "" {
		return errStr
	}
	if c.businessID == nil {
		return fmt.Sprintf("error: %s has no linked business record.", c.name)
	}
	score := s.leadCtx.LeadScoreJSON(ctx, userID, *c.businessID)
	trade := s.leadCtx.ShipmentContext(ctx, *c.businessID)
	business := s.leadCtx.BusinessJSON(ctx, *c.businessID)
	if score == "" {
		score = "(not scored)"
	}
	if trade == "" {
		trade = "(no shipment data on file)"
	}
	return fmt.Sprintf("Lead: %s\nLead score: %s\nTrade history: %s\nBusiness: %s",
		c.name, score, trade, truncate(business, agentBodyMax))
}

func (s *Service) toolTopLeads(ctx context.Context, userID, campaignID uuid.UUID, args json.RawMessage) string {
	var a struct {
		Dimension string `json:"dimension"`
		Limit     int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &a)
	col, ok := leadScoreColumns[strings.TrimSpace(a.Dimension)]
	if !ok {
		return "error: dimension must be one of overall, purchase_likelihood, deal_size_potential, urgency_score, fit_score, accessibility_score."
	}
	limit := a.Limit
	if limit <= 0 {
		limit = 10
	}
	if limit > 25 {
		limit = 25
	}
	// col is whitelisted above, safe to interpolate.
	q := fmt.Sprintf(
		`SELECT ct.display_name, COALESCE(b.name,''), ls.%s
		   FROM campaign_contacts cc
		   JOIN contacts ct ON ct.id = cc.contact_id
		   LEFT JOIN businesses b ON b.id = ct.business_id
		   JOIN lead_scores ls ON ls.business_id = ct.business_id AND ls.user_id = $2
		  WHERE cc.campaign_id = $1 AND ls.%s IS NOT NULL
		  ORDER BY ls.%s DESC NULLS LAST LIMIT $3`, col, col, col)
	rows, err := s.pool.Query(ctx, q, campaignID, userID, limit)
	if err != nil {
		return "error: could not rank leads."
	}
	defer rows.Close()
	var b strings.Builder
	n := 0
	for rows.Next() {
		var name, biz string
		var val *int
		if rows.Scan(&name, &biz, &val) != nil {
			continue
		}
		v := "?"
		if val != nil {
			v = fmt.Sprintf("%d", *val)
		}
		line := fmt.Sprintf("- %s (%s)", name, v)
		if biz != "" {
			line = fmt.Sprintf("- %s [%s] (%s)", name, biz, v)
		}
		b.WriteString(line + "\n")
		n++
	}
	if n == 0 {
		return "No scored leads in this group for that dimension."
	}
	return fmt.Sprintf("Top %d by %s:\n%s", n, col, strings.TrimSpace(b.String()))
}

func (s *Service) toolGetBrand(ctx context.Context, camp *domain.Campaign) string {
	sp, _ := s.resolveBrand(ctx, camp)
	if sp == nil {
		return "error: no brand is configured for this group."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Brand: %s\n", sp.CompanyName)
	add := func(label, val string) {
		if strings.TrimSpace(val) != "" {
			fmt.Fprintf(&b, "%s: %s\n", label, truncate(val, 400))
		}
	}
	add("Product", sp.ProductDescription)
	add("Value proposition", sp.ValueProp)
	add("Target buyer", sp.TargetBuyerDescription)
	add("Tone", sp.Tone)
	add("Target industries", sp.TargetIndustries)
	add("Target countries", sp.TargetCountries)
	add("Deal-breakers", sp.DealBreakers)
	add("Competitive moats", sp.CompetitiveMoats)
	if sp.TypicalDealSizeUSD != nil {
		fmt.Fprintf(&b, "Typical deal size: $%d\n", *sp.TypicalDealSizeUSD)
	}
	if ok, name := senderCatalogIfPresent(sp); ok {
		fmt.Fprintf(&b, "Catalog on file: %s\n", name)
	} else {
		b.WriteString("Catalog on file: none\n")
	}
	add("Signature", sp.Signature)
	return strings.TrimSpace(b.String())
}

func (s *Service) toolCampaignSetup(ctx context.Context, camp *domain.Campaign) string {
	var b strings.Builder
	goal := camp.Goal
	if o := s.parsePositioning(camp); o != nil && o.Goal != "" {
		goal = o.Goal
	}
	fmt.Fprintf(&b, "Goal: %s\n", d(goal, "(none set)"))
	fmt.Fprintf(&b, "On positive reply: %s\n", d(camp.OnPositiveAction, "(default)"))
	fmt.Fprintf(&b, "On negative reply: %s\n", d(camp.OnNegativeAction, "(default)"))
	if camp.SequenceID == nil {
		b.WriteString("Follow-up cadence: none")
		return strings.TrimSpace(b.String())
	}
	rows, err := s.pool.Query(ctx,
		`SELECT step_number, wait_days, trigger, action, auto_send
		   FROM sequence_steps WHERE sequence_id=$1 ORDER BY step_number`, *camp.SequenceID)
	if err != nil {
		b.WriteString("Follow-up cadence: (unavailable)")
		return strings.TrimSpace(b.String())
	}
	defer rows.Close()
	b.WriteString("Follow-up cadence:\n")
	n := 0
	for rows.Next() {
		var stepNo, waitDays int
		var trigger, action string
		var autoSend bool
		if rows.Scan(&stepNo, &waitDays, &trigger, &action, &autoSend) != nil {
			continue
		}
		mode := "drafts for approval"
		if autoSend {
			mode = "auto-send"
		}
		fmt.Fprintf(&b, "- step %d: wait %dd, on %s → %s (%s)\n", stepNo, waitDays, trigger, action, mode)
		n++
	}
	if n == 0 {
		b.WriteString("- (no steps)")
	}
	return strings.TrimSpace(b.String())
}

// renderDraftList renders list_drafts rows (name + subject + snippet).
func renderDraftList(rows interface {
	Next() bool
	Scan(...any) error
	Close()
}, label string) string {
	defer rows.Close()
	var b strings.Builder
	n := 0
	for rows.Next() {
		var name, subj, body string
		if rows.Scan(&name, &subj, &body) != nil {
			continue
		}
		fmt.Fprintf(&b, "- %s | %s | %s\n", name, subj, truncate(body, agentSnippetMax))
		n++
	}
	if n == 0 {
		return "No " + label + "."
	}
	return fmt.Sprintf("%d %s:\n%s", n, label, strings.TrimSpace(b.String()))
}

// d returns s, or fallback when s is empty.
func d(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
