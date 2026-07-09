// Package group is the "Email Groups" facade. An Email Group is the
// user-facing name for a campaign plus its attached follow-up sequence —
// this package owns no tables of its own, it orchestrates the existing
// campaign + sequence + contact services and reads market/brand context.
package group

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/campaign"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/contactgroup"
	"github.com/batuhan/marketintel/internal/outreach/events"
	"github.com/batuhan/marketintel/internal/outreach/sequence"
)

// ErrGroupNoBrand is returned when creating an email group from a contact
// group that has no sender profile (brand) assigned yet.
var ErrGroupNoBrand = errors.New("contact group has no brand assigned — set a brand on the contact group first")

type Service struct {
	campaignSvc      *campaign.Service
	campaignRepo     campaign.Repository
	sequenceSvc      *sequence.Service
	contactRepo      contact.Repository
	contactGroupRepo contactgroup.Repository
	pool             *pgxpool.Pool
	broker           *events.Broker
}

func NewService(
	campaignSvc *campaign.Service,
	campaignRepo campaign.Repository,
	sequenceSvc *sequence.Service,
	contactRepo contact.Repository,
	contactGroupRepo contactgroup.Repository,
	pool *pgxpool.Pool,
) *Service {
	return &Service{
		campaignSvc:      campaignSvc,
		campaignRepo:     campaignRepo,
		sequenceSvc:      sequenceSvc,
		contactRepo:      contactRepo,
		contactGroupRepo: contactGroupRepo,
		pool:             pool,
	}
}

// GroupSummary is a campaign presented as an Email Group, enriched with its
// market + brand names for the list view.
type GroupSummary struct {
	*domain.CampaignSummary
	ContactGroupID   *uuid.UUID `json:"contact_group_id"`
	ContactGroupName string     `json:"contact_group_name"`
	BrandID          *uuid.UUID `json:"brand_id"`
	BrandName        string     `json:"brand_name"`
	// SenderEmail is the from-address of the group's channel (channel_id comes
	// from the embedded Campaign). Surfaces which account the group sends from.
	SenderEmail string `json:"sender_email"`
}

// GroupDetail adds the follow-up steps, per-facet counts, and outstanding
// notifications to the summary. FacetCounts is keyed by facet value (sentiment
// level, status key, or intent tag).
type GroupDetail struct {
	*GroupSummary
	Steps         []domain.SequenceStep `json:"steps"`
	FacetCounts   map[string]int        `json:"facet_counts"`
	Notifications []domain.Task         `json:"notifications"`
}

// GroupEmail is one conversation row inside a group, shaped for the email
// list pane.
type GroupEmail struct {
	ContactID      uuid.UUID  `json:"contact_id"`
	ContactName    string     `json:"contact_name"`
	ContactEmail   *string    `json:"contact_email"`
	BusinessName   string     `json:"business_name"`
	ConversationID *uuid.UUID `json:"conversation_id"`
	Subject        *string    `json:"subject"`
	Status         string     `json:"status"`
	LastDirection  *string    `json:"last_direction"`
	LastMessageAt  *string    `json:"last_message_at"`
	Sentiment      *string    `json:"sentiment"`        // legacy 3-value (back-compat)
	SentimentScore *float64   `json:"sentiment_score"`  // -1.0..1.0
	SentimentLabel *string    `json:"sentiment_label"`  // 5-level derived label
	Tags           []string   `json:"tags"`             // intent tags
	Unread         bool       `json:"unread"`
	// HasPendingDraft surfaces engine/manual drafts awaiting approval so they
	// are findable from the group list (they only render inside the thread).
	HasPendingDraft bool `json:"has_pending_draft"`
	// ScheduledSendAt is when the cold opener is set to go out (for approved,
	// not-yet-sent rows) — so "Scheduled" shows WHEN, not the draft's date.
	ScheduledSendAt *string `json:"scheduled_send_at"`
	// SkipReason says WHY a row is failed/skipped (e.g. "sending account
	// isn't fully connected") so the user can act instead of guessing.
	SkipReason *string `json:"skip_reason"`
	// CurrentStep / NextRunAt expose where this contact is in the follow-up
	// cadence (from its sequence run): the next step to fire and when.
	CurrentStep *int    `json:"current_step"`
	NextRunAt   *string `json:"next_run_at"`
}

// EmailFacets is the multi-select filter applied to a group's email list:
// OR within a facet, AND across facets. Empty facet = no constraint.
type EmailFacets struct {
	Sentiment []string // 5-level labels
	Tags      []string // intent tags
	Status    []string // status keys: replied / no_reply / cold / advanced
}

// CreateGroupInput is the one-call create payload. An email group is built
// from a contact group: it inherits that group's brand and members.
type CreateGroupInput struct {
	Name             string                `json:"name"`
	Goal             string                `json:"goal"`
	ContactGroupID   uuid.UUID             `json:"contact_group_id"`
	SendPacePerDay   int                   `json:"send_pace_per_day"`
	AttachCatalog    bool                  `json:"attach_catalog"`
	OnPositiveAction string                `json:"on_positive_action"`
	OnNegativeAction string                `json:"on_negative_action"`
	Steps            []domain.SequenceStep `json:"steps"`
	// ChannelID is the email account the group sends from. Optional — falls back
	// to the brand default, then any enabled account.
	ChannelID *uuid.UUID `json:"channel_id"`
}

// List returns the user's email groups (campaigns), newest first, enriched
// with market + brand names and per-status counts.
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]GroupSummary, error) {
	camps, err := s.campaignRepo.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]GroupSummary, 0, len(camps))
	for i := range camps {
		gs, err := s.summaryFor(ctx, userID, &camps[i])
		if err != nil {
			return nil, err
		}
		out = append(out, *gs)
	}
	return out, nil
}

func (s *Service) summaryFor(ctx context.Context, userID uuid.UUID, camp *domain.Campaign) (*GroupSummary, error) {
	sum, err := s.campaignRepo.Summary(ctx, userID, camp.ID)
	if err != nil {
		return nil, err
	}
	gs := &GroupSummary{
		CampaignSummary: sum,
		ContactGroupID:  camp.ContactGroupID,
		BrandID:         camp.SenderProfileID,
	}
	if camp.ContactGroupID != nil {
		_ = s.pool.QueryRow(ctx,
			`SELECT name FROM contact_groups WHERE id = $1`, *camp.ContactGroupID,
		).Scan(&gs.ContactGroupName)
	}
	if camp.SenderProfileID != nil {
		_ = s.pool.QueryRow(ctx,
			`SELECT name FROM sender_profiles WHERE id = $1`, *camp.SenderProfileID,
		).Scan(&gs.BrandName)
	}
	if camp.ChannelID != uuid.Nil {
		_ = s.pool.QueryRow(ctx,
			`SELECT from_email FROM user_channels WHERE id = $1`, camp.ChannelID,
		).Scan(&gs.SenderEmail)
	}
	return gs, nil
}

// Get returns the full group detail.
func (s *Service) Get(ctx context.Context, userID, id uuid.UUID) (*GroupDetail, error) {
	camp, err := s.campaignRepo.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	gs, err := s.summaryFor(ctx, userID, camp)
	if err != nil {
		return nil, err
	}
	detail := &GroupDetail{GroupSummary: gs, FacetCounts: map[string]int{}}

	if camp.SequenceID != nil {
		seq, err := s.sequenceSvc.GetWithSteps(ctx, userID, *camp.SequenceID)
		if err == nil && seq != nil {
			detail.Steps = seq.Steps
		}
	}

	counts, err := s.facetCounts(ctx, id)
	if err != nil {
		return nil, err
	}
	detail.FacetCounts = counts

	notes, err := s.notifications(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	detail.Notifications = notes
	return detail, nil
}

// SetBroker wires the SSE broker after construction (main.go creates the
// broker later than this service — same pattern as campaign.SetSequenceStarter).
func (s *Service) SetBroker(b *events.Broker) { s.broker = b }

// --- Draft assistant chat (facade over campaign service) ---

func (s *Service) AssistantHistory(ctx context.Context, userID, id uuid.UUID) ([]campaign.AssistantMessage, error) {
	return s.campaignSvc.AssistantHistory(ctx, userID, id)
}

// AssistantSend runs one agent turn, streaming each tool the agent calls as an
// "assistant_thinking" progress event so the dock can show live status.
func (s *Service) AssistantSend(ctx context.Context, userID, id uuid.UUID, text string) (*campaign.AssistantMessage, error) {
	onTool := func(name string) {
		if s.broker == nil {
			return
		}
		campID := id
		s.broker.Publish(events.Event{
			Kind:       events.KindCampaignProgress,
			UserID:     userID,
			CampaignID: &campID,
			Data:       map[string]any{"action": "assistant_thinking", "tool": name},
		})
	}
	return s.campaignSvc.AssistantSend(ctx, userID, id, text, onTool)
}

// AssistantConfirm executes a proposed action (edit drafts / playbook change),
// streaming per-draft progress over SSE for draft edits.
func (s *Service) AssistantConfirm(ctx context.Context, userID, id, messageID uuid.UUID) (int, int, error) {
	return s.campaignSvc.AssistantConfirm(ctx, userID, id, messageID, func(done, total int) {
		if s.broker == nil {
			return
		}
		campID := id
		s.broker.Publish(events.Event{
			Kind:       events.KindCampaignProgress,
			UserID:     userID,
			CampaignID: &campID,
			Data:       map[string]any{"action": "directive", "done": done, "total": total},
		})
	})
}

func (s *Service) AssistantDismiss(ctx context.Context, userID, id, messageID uuid.UUID) error {
	return s.campaignSvc.AssistantDismiss(ctx, userID, id, messageID)
}

// PlaybookEntry is one authored per-tag reply instruction for a group.
type PlaybookEntry struct {
	Tag         string `json:"tag"`
	Instruction string `json:"instruction"`
}

// GetPlaybook returns the group's authored per-tag reply guidance.
func (s *Service) GetPlaybook(ctx context.Context, userID, id uuid.UUID) ([]PlaybookEntry, error) {
	if _, err := s.campaignRepo.Get(ctx, userID, id); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT tag, instruction FROM campaign_tag_guidance WHERE campaign_id=$1 ORDER BY tag`, id)
	if err != nil {
		return nil, fmt.Errorf("list playbook: %w", err)
	}
	defer rows.Close()
	out := []PlaybookEntry{}
	for rows.Next() {
		var e PlaybookEntry
		if err := rows.Scan(&e.Tag, &e.Instruction); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

// SavePlaybook replaces the group's per-tag reply guidance. Empty instructions
// and unknown tags are dropped.
func (s *Service) SavePlaybook(ctx context.Context, userID, id uuid.UUID, entries []PlaybookEntry) error {
	if _, err := s.campaignRepo.Get(ctx, userID, id); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM campaign_tag_guidance WHERE campaign_id=$1`, id); err != nil {
		return err
	}
	for _, e := range entries {
		if !domain.IsValidIntentTag(e.Tag) {
			return fmt.Errorf("unknown intent tag %q", e.Tag)
		}
		if strings.TrimSpace(e.Instruction) == "" {
			// Empty rows are editor placeholders — skipped, and the handler
			// returns the saved set so the UI reflects exactly what persisted.
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO campaign_tag_guidance (campaign_id, tag, instruction) VALUES ($1,$2,$3)
			 ON CONFLICT (campaign_id, tag) DO UPDATE SET instruction=EXCLUDED.instruction, updated_at=now()`,
			id, e.Tag, strings.TrimSpace(e.Instruction)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Create builds the campaign + sequence + contact set in one call.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateGroupInput) (*GroupSummary, error) {
	if in.Name == "" {
		return nil, errors.New("group name is required")
	}
	if in.ContactGroupID == uuid.Nil {
		return nil, errors.New("contact_group_id is required")
	}

	// Resolve the contact group's brand. No brand → can't create a group.
	cg, err := s.contactGroupRepo.Get(ctx, userID, in.ContactGroupID)
	if err != nil {
		return nil, err
	}
	if cg.SenderProfileID == nil {
		return nil, ErrGroupNoBrand
	}
	brandID := cg.SenderProfileID

	// Which account the group sends from: the user's explicit pick wins; else
	// the brand's default channel. CreateCampaign validates it and falls back to
	// any enabled account if neither is set/valid.
	channelID := in.ChannelID
	if channelID == nil {
		_ = s.pool.QueryRow(ctx,
			`SELECT default_channel_id FROM sender_profiles WHERE id = $1`, *brandID,
		).Scan(&channelID)
	}

	camp := domain.Campaign{
		UserID:           userID,
		Name:             in.Name,
		Goal:             in.Goal,
		ContactGroupID:   &in.ContactGroupID,
		SenderProfileID:  brandID,
		SendPacePerDay:   in.SendPacePerDay,
		AttachCatalog:    in.AttachCatalog,
		OnPositiveAction: in.OnPositiveAction,
		OnNegativeAction: in.OnNegativeAction,
	}
	if channelID != nil {
		camp.ChannelID = *channelID
	}
	created, err := s.campaignSvc.CreateCampaign(ctx, userID, camp)
	if err != nil {
		return nil, err
	}

	// Attach follow-up steps as a sequence, link it back onto the campaign.
	if len(in.Steps) > 0 {
		seq, err := s.sequenceSvc.CreateWithSteps(ctx, userID, sequence.CreateSequenceInput{
			Name:  in.Name + " — follow-ups",
			Steps: in.Steps,
		})
		if err != nil {
			return nil, fmt.Errorf("create follow-up steps: %w", err)
		}
		created.SequenceID = &seq.ID
		if _, err := s.campaignRepo.Update(ctx, *created); err != nil {
			return nil, fmt.Errorf("link follow-ups: %w", err)
		}
	}

	// Pull the contact group's members and add them to the campaign.
	contacts, _, err := s.contactRepo.ListByGroup(ctx, userID, in.ContactGroupID, 100000, 0)
	if err != nil {
		return nil, fmt.Errorf("load contact group members: %w", err)
	}
	if len(contacts) > 0 {
		ids := make([]uuid.UUID, 0, len(contacts))
		for _, c := range contacts {
			ids = append(ids, c.ID)
		}
		if _, err := s.campaignSvc.AddContacts(ctx, userID, created.ID, ids, nil, false); err != nil {
			return nil, fmt.Errorf("add contacts: %w", err)
		}
	}

	return s.summaryFor(ctx, userID, created)
}

// ReplaceSteps edits a group's follow-up steps. Creates the backing sequence
// on first use if the group had none.
func (s *Service) ReplaceSteps(ctx context.Context, userID, id uuid.UUID, steps []domain.SequenceStep) ([]domain.SequenceStep, error) {
	camp, err := s.campaignRepo.Get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if camp.SequenceID == nil {
		seq, err := s.sequenceSvc.CreateWithSteps(ctx, userID, sequence.CreateSequenceInput{
			Name:  camp.Name + " — follow-ups",
			Steps: steps,
		})
		if err != nil {
			return nil, err
		}
		camp.SequenceID = &seq.ID
		if _, err := s.campaignRepo.Update(ctx, *camp); err != nil {
			return nil, err
		}
		return seq.Steps, nil
	}
	return s.sequenceSvc.ReplaceSteps(ctx, userID, *camp.SequenceID, steps)
}

// SetChannel changes which email account a group sends from. Allowed only before
// launch (draft/ready) — once live, threads + reply polling belong to that
// mailbox, so the account is locked.
func (s *Service) SetChannel(ctx context.Context, userID, groupID, channelID uuid.UUID) error {
	camp, err := s.campaignRepo.Get(ctx, userID, groupID)
	if err != nil {
		return err
	}
	if camp.Status != domain.CampaignStatusDraft && camp.Status != domain.CampaignStatusReady {
		return errors.New("the sending account can only be changed before the group is launched")
	}
	var ch domain.UserChannel
	if err := s.pool.QueryRow(ctx,
		`SELECT type, from_email, enabled, config_encrypted, oauth_refresh_token_encrypted
		 FROM user_channels WHERE id=$1 AND user_id=$2`, channelID, userID,
	).Scan(&ch.Type, &ch.FromEmail, &ch.Enabled, &ch.ConfigCipher, &ch.OAuthRefreshTokenCipher); err != nil {
		return errors.New("email account not found")
	}
	if !ch.Enabled {
		return errors.New("that email account is disconnected — reconnect it first")
	}
	if !ch.SendReady() {
		return fmt.Errorf("%s isn't fully connected — reconnect it in Settings → Email channels", ch.FromEmail)
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE campaigns SET channel_id=$1, updated_at=now() WHERE id=$2 AND user_id=$3`,
		channelID, groupID, userID)
	return err
}

// Emails returns the conversations in a group, filtered by facets (OR within a
// facet, AND across facets). Empty facets return every row.
func (s *Service) Emails(ctx context.Context, userID, id uuid.UUID, f EmailFacets) ([]GroupEmail, error) {
	// Ownership check.
	if _, err := s.campaignRepo.Get(ctx, userID, id); err != nil {
		return nil, err
	}
	args := []any{id}
	q := `SELECT cc.contact_id, ct.display_name, ct.primary_email,
	             COALESCE(b.name, ''), cv.id, cv.subject, cc.status,
	             cv.last_direction, cv.last_message_at, cv.last_inbound_sentiment,
	             cv.last_inbound_sentiment_score, cv.last_inbound_sentiment_label,
	             COALESCE(ARRAY(SELECT tag FROM conversation_tags WHERE conversation_id = cv.id ORDER BY tag), '{}'),
	             COALESCE(cv.unread, false),
	             EXISTS(SELECT 1 FROM messages pm WHERE pm.conversation_id = cv.id AND pm.status = 'pending_approval'),
	             cc.scheduled_send_at, cc.skip_reason, sr.current_step,
	             CASE WHEN sr.status = 'active' THEN sr.next_run_at ELSE NULL END
	      FROM campaign_contacts cc
	      JOIN contacts ct ON ct.id = cc.contact_id
	      LEFT JOIN businesses b ON b.id = ct.business_id
	      LEFT JOIN conversations cv ON cv.id = cc.conversation_id
	      LEFT JOIN sequence_runs sr ON sr.conversation_id = cv.id
	      WHERE cc.campaign_id = $1`
	q += buildFacetWhere(f, &args)
	q += ` ORDER BY cv.last_message_at DESC NULLS LAST, ct.display_name`

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list group emails: %w", err)
	}
	defer rows.Close()

	out := []GroupEmail{}
	for rows.Next() {
		var e GroupEmail
		var lastAt, schedAt, nextRunAt *time.Time
		if err := rows.Scan(&e.ContactID, &e.ContactName, &e.ContactEmail,
			&e.BusinessName, &e.ConversationID, &e.Subject, &e.Status,
			&e.LastDirection, &lastAt, &e.Sentiment,
			&e.SentimentScore, &e.SentimentLabel, &e.Tags, &e.Unread, &e.HasPendingDraft,
			&schedAt, &e.SkipReason, &e.CurrentStep, &nextRunAt); err != nil {
			return nil, fmt.Errorf("scan group email: %w", err)
		}
		// timestamptz must be scanned as time.Time (pgx binary mode rejects
		// *string) and formatted for the JSON shape the frontend expects.
		if lastAt != nil {
			s := lastAt.UTC().Format(time.RFC3339)
			e.LastMessageAt = &s
		}
		if schedAt != nil {
			s := schedAt.UTC().Format(time.RFC3339)
			e.ScheduledSendAt = &s
		}
		if nextRunAt != nil {
			s := nextRunAt.UTC().Format(time.RFC3339)
			e.NextRunAt = &s
		}
		out = append(out, e)
	}
	return out, nil
}

// --- Cadence observability ---

// FunnelStep is one touch in the cadence funnel: step 0 = cold opener, 1..N =
// follow-ups. Sent is the number of contacts that received this touch.
type FunnelStep struct {
	Step  int     `json:"step"`
	Label string  `json:"label"`
	Sent  int     `json:"sent"`
	Pct   float64 `json:"pct"`
}

// UpcomingDay is the count of sends queued for a given day.
type UpcomingDay struct {
	Date     string `json:"date"` // YYYY-MM-DD
	Cold     int    `json:"cold"`
	Followup int    `json:"followup"`
}

// CadenceStats is the follow-up sequence dashboard for a group.
type CadenceStats struct {
	TotalContacts int            `json:"total_contacts"`
	Funnel        []FunnelStep   `json:"funnel"`
	Exits         map[string]int `json:"exits"`
	CompletionPct float64        `json:"completion_pct"`
	Upcoming      []UpcomingDay  `json:"upcoming"`
}

// Cadence returns the funnel, exit breakdown, completion %, and upcoming sends
// for a group — "which touch fired, what's next, how far along, why people left."
func (s *Service) Cadence(ctx context.Context, userID, id uuid.UUID) (*CadenceStats, error) {
	if _, err := s.campaignRepo.Get(ctx, userID, id); err != nil {
		return nil, err
	}
	out := &CadenceStats{Exits: map[string]int{}, Funnel: []FunnelStep{}, Upcoming: []UpcomingDay{}}

	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM campaign_contacts WHERE campaign_id=$1`, id).Scan(&out.TotalContacts)
	total := float64(out.TotalContacts)
	pct := func(n int) float64 {
		if total == 0 {
			return 0
		}
		return float64(int(float64(n)/total*1000+0.5)) / 10
	}

	// Funnel: cold opener (messages with no sequence step) + each follow-up step.
	var coldSent int
	_ = s.pool.QueryRow(ctx,
		`SELECT count(DISTINCT m.conversation_id) FROM messages m
		   JOIN conversations cv ON cv.id = m.conversation_id
		  WHERE cv.campaign_id=$1 AND m.direction='out' AND m.status='sent' AND m.sequence_step_id IS NULL`,
		id).Scan(&coldSent)
	out.Funnel = append(out.Funnel, FunnelStep{Step: 0, Label: "Cold opener", Sent: coldSent, Pct: pct(coldSent)})
	if rows, err := s.pool.Query(ctx,
		`SELECT ss.step_number, count(DISTINCT m.conversation_id) FROM messages m
		   JOIN sequence_steps ss ON ss.id = m.sequence_step_id
		   JOIN conversations cv ON cv.id = m.conversation_id
		  WHERE cv.campaign_id=$1 AND m.direction='out' AND m.status='sent'
		  GROUP BY ss.step_number ORDER BY ss.step_number`, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var step, sent int
			if rows.Scan(&step, &sent) == nil {
				out.Funnel = append(out.Funnel, FunnelStep{Step: step, Label: fmt.Sprintf("Follow-up %d", step), Sent: sent, Pct: pct(sent)})
			}
		}
	}

	// Exit breakdown.
	if rows, err := s.pool.Query(ctx,
		`SELECT COALESCE(last_inbound_sentiment,'neutral'), count(*) FROM conversations
		  WHERE campaign_id=$1 AND last_direction='in' GROUP BY 1`, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var sent string
			var n int
			if rows.Scan(&sent, &n) == nil {
				out.Exits["replied_"+sent] = n
			}
		}
	}
	var cold, failed int
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM campaign_contacts WHERE campaign_id=$1 AND status='cold'`, id).Scan(&cold)
	_ = s.pool.QueryRow(ctx, `SELECT count(*) FROM campaign_contacts WHERE campaign_id=$1 AND status='failed'`, id).Scan(&failed)
	out.Exits["cold"] = cold
	out.Exits["failed"] = failed
	replied := out.Exits["replied_positive"] + out.Exits["replied_negative"] + out.Exits["replied_neutral"]
	terminal := replied + cold + failed
	out.Exits["in_progress"] = out.TotalContacts - terminal
	out.CompletionPct = pct(terminal)

	// Upcoming sends (next 14 days), cold + follow-up, merged by date.
	upMap := map[string]*UpcomingDay{}
	day := func(d string) *UpcomingDay {
		if u, ok := upMap[d]; ok {
			return u
		}
		u := &UpcomingDay{Date: d}
		upMap[d] = u
		return u
	}
	if rows, err := s.pool.Query(ctx,
		`SELECT to_char(scheduled_send_at AT TIME ZONE 'UTC','YYYY-MM-DD'), count(*)
		   FROM campaign_contacts
		  WHERE campaign_id=$1 AND status='approved' AND scheduled_send_at > now()
		  GROUP BY 1 ORDER BY 1 LIMIT 14`, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var d string
			var n int
			if rows.Scan(&d, &n) == nil {
				day(d).Cold += n
			}
		}
	}
	if rows, err := s.pool.Query(ctx,
		`SELECT to_char(sr.next_run_at AT TIME ZONE 'UTC','YYYY-MM-DD'), count(*)
		   FROM sequence_runs sr JOIN conversations cv ON cv.id = sr.conversation_id
		  WHERE cv.campaign_id=$1 AND sr.status='active' AND sr.next_run_at > now()
		  GROUP BY 1 ORDER BY 1 LIMIT 14`, id); err == nil {
		defer rows.Close()
		for rows.Next() {
			var d string
			var n int
			if rows.Scan(&d, &n) == nil {
				day(d).Followup += n
			}
		}
	}
	dates := make([]string, 0, len(upMap))
	for d := range upMap {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	for _, d := range dates {
		out.Upcoming = append(out.Upcoming, *upMap[d])
	}

	return out, nil
}

// sentimentLevelExpr is the facet expression for a conversation's sentiment
// level: the 5-level label when present, else the legacy 3-value sentiment
// (whose values are valid level names) so pre-scoring rows still match the
// facet their badge displays.
const sentimentLevelExpr = "COALESCE(cv.last_inbound_sentiment_label, cv.last_inbound_sentiment)"

// buildFacetWhere appends array args and returns the SQL predicate for the
// facets: OR within a facet, AND across facets. Returns "" when no facet is set.
func buildFacetWhere(f EmailFacets, args *[]any) string {
	var clauses []string
	if len(f.Sentiment) > 0 {
		*args = append(*args, f.Sentiment)
		clauses = append(clauses, fmt.Sprintf(sentimentLevelExpr+" = ANY($%d)", len(*args)))
	}
	if len(f.Tags) > 0 {
		*args = append(*args, f.Tags)
		clauses = append(clauses, fmt.Sprintf(
			"cv.id IN (SELECT conversation_id FROM conversation_tags WHERE tag = ANY($%d))", len(*args)))
	}
	if len(f.Status) > 0 {
		var preds []string
		for _, st := range f.Status {
			if p := statusPredicate(st); p != "" {
				preds = append(preds, p)
			}
		}
		if len(preds) > 0 {
			clauses = append(clauses, "("+strings.Join(preds, " OR ")+")")
		}
	}
	if len(clauses) == 0 {
		return ""
	}
	return " AND " + strings.Join(clauses, " AND ")
}

// statusPredicate maps a status facet key to a SQL fragment (whitelist-only).
// "advanced" keys off contacts.pipeline_stage since conversations has no stage
// column. "replied" excludes cold contacts so the chips don't double-count a
// negative-replied row under both Replied and Cold. Unknown keys yield FALSE —
// an unrecognized filter must narrow to nothing, never silently widen.
func statusPredicate(key string) string {
	switch key {
	case "replied":
		return "((cc.status = 'replied' OR cv.last_direction = 'in') AND cc.status <> 'cold')"
	case "no_reply":
		return "(cc.status = 'sent' AND cv.last_direction = 'out')"
	case "cold":
		return "cc.status = 'cold'"
	case "advanced":
		return "ct.pipeline_stage IN ('qualified','won')"
	default:
		return "FALSE"
	}
}

// facetCounts returns the count for every facet value in the group in two
// queries: one conditional-aggregate over sentiment levels + statuses, and one
// GROUP BY over intent tags.
func (s *Service) facetCounts(ctx context.Context, campaignID uuid.UUID) (map[string]int, error) {
	out := map[string]int{}

	// The FILTER expressions MUST match buildFacetWhere/statusPredicate exactly,
	// or a chip's count disagrees with the rows its filter returns.
	var veryNeg, neg, neutral, pos, veryPos, replied, noReply, cold, advanced int
	err := s.pool.QueryRow(ctx, `
		SELECT
		  count(*) FILTER (WHERE `+sentimentLevelExpr+` = 'very_negative'),
		  count(*) FILTER (WHERE `+sentimentLevelExpr+` = 'negative'),
		  count(*) FILTER (WHERE `+sentimentLevelExpr+` = 'neutral'),
		  count(*) FILTER (WHERE `+sentimentLevelExpr+` = 'positive'),
		  count(*) FILTER (WHERE `+sentimentLevelExpr+` = 'very_positive'),
		  count(*) FILTER (WHERE (cc.status = 'replied' OR cv.last_direction = 'in') AND cc.status <> 'cold'),
		  count(*) FILTER (WHERE cc.status = 'sent' AND cv.last_direction = 'out'),
		  count(*) FILTER (WHERE cc.status = 'cold'),
		  count(*) FILTER (WHERE ct.pipeline_stage IN ('qualified','won'))
		FROM campaign_contacts cc
		JOIN contacts ct ON ct.id = cc.contact_id
		LEFT JOIN conversations cv ON cv.id = cc.conversation_id
		WHERE cc.campaign_id = $1`, campaignID,
	).Scan(&veryNeg, &neg, &neutral, &pos, &veryPos, &replied, &noReply, &cold, &advanced)
	if err != nil {
		return nil, fmt.Errorf("facet counts: %w", err)
	}
	out["very_negative"], out["negative"], out["neutral"] = veryNeg, neg, neutral
	out["positive"], out["very_positive"] = pos, veryPos
	out["replied"], out["no_reply"], out["cold"], out["advanced"] = replied, noReply, cold, advanced

	rows, err := s.pool.Query(ctx, `
		SELECT tag, count(*) FROM conversation_tags
		WHERE conversation_id IN (
		  SELECT conversation_id FROM campaign_contacts
		  WHERE campaign_id = $1 AND conversation_id IS NOT NULL
		)
		GROUP BY tag`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("facet tag counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var tag string
		var n int
		if err := rows.Scan(&tag, &n); err != nil {
			return nil, err
		}
		out[tag] = n
	}
	return out, nil
}

func (s *Service) notifications(ctx context.Context, userID, campaignID uuid.UUID) ([]domain.Task, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT t.id, t.user_id, t.contact_id, t.conversation_id, t.title, t.body,
		        t.due_at, t.completed_at, t.created_at, t.updated_at
		 FROM tasks t
		 WHERE t.user_id = $1
		   AND t.completed_at IS NULL
		   AND t.conversation_id IN (
		     SELECT conversation_id FROM campaign_contacts
		     WHERE campaign_id = $2 AND conversation_id IS NOT NULL
		   )
		 ORDER BY t.created_at DESC`,
		userID, campaignID,
	)
	if err != nil {
		return nil, fmt.Errorf("load notifications: %w", err)
	}
	defer rows.Close()
	out := []domain.Task{}
	for rows.Next() {
		var t domain.Task
		if err := rows.Scan(&t.ID, &t.UserID, &t.ContactID, &t.ConversationID,
			&t.Title, &t.Body, &t.DueAt, &t.CompletedAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		out = append(out, t)
	}
	return out, nil
}
