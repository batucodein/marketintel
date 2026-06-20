package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/events"
	"github.com/batuhan/marketintel/internal/outreach/leadctx"
	"github.com/batuhan/marketintel/internal/outreach/replyguidance"
	"github.com/batuhan/marketintel/internal/outreach/sender"
	"github.com/batuhan/marketintel/internal/outreach/sequence"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
	"github.com/batuhan/marketintel/internal/scoring"
)

// Service orchestrates an AI-persona simulation. It reuses the real draft/
// reply/sentiment prompts + models + guard, the real lead context loaders,
// and the shared pure trigger evaluator — but runs entirely in memory and
// writes only to the simulations tables. It never sends email.
type Service struct {
	repo    Repository
	scoring scoring.Repository
	leadCtx *leadctx.Loader
	sender  sender.Repository
	ai      *ai.Router
	broker  *events.Broker
}

func NewService(repo Repository, scoringRepo scoring.Repository, leadCtxLoader *leadctx.Loader, senderRepo sender.Repository, aiRouter *ai.Router, broker *events.Broker) *Service {
	return &Service{repo: repo, scoring: scoringRepo, leadCtx: leadCtxLoader, sender: senderRepo, ai: aiRouter, broker: broker}
}

type RunInput struct {
	Name             string                `json:"name"`
	// Mode selects the stepped, interactive engine ("interactive", default) or
	// the one-shot batch ("quick", no pauses — the original behaviour).
	Mode             string                `json:"mode"`
	MarketID         uuid.UUID             `json:"market_id"`
	BrandID          uuid.UUID             `json:"brand_id"`
	LeadCount        int                   `json:"lead_count"`
	Personas         []string              `json:"personas"`
	Steps            []domain.SequenceStep `json:"steps"`
	OnPositiveAction string                `json:"on_positive_action"`
	OnNegativeAction string                `json:"on_negative_action"`
	// Playbook is a transient per-tag reply guidance map (tag → instruction)
	// passed in so the Lab can test how the playbook shapes drafts. Not stored.
	Playbook map[string]string `json:"playbook"`
	// IncludeLessons controls whether the brand's remembered reply lessons are
	// injected into reply drafts (production always injects them). Pointer so
	// an absent field defaults to true — nil means "mirror production".
	IncludeLessons *bool `json:"include_lessons"`
}

type Summary struct {
	LeadCount         int            `json:"lead_count"`
	Outcomes          map[string]int `json:"outcomes"`
	AvgScore          float64        `json:"avg_score"`
	AvgHumanFeel      float64        `json:"avg_human_feel"`
	SentimentAccuracy *float64       `json:"sentiment_accuracy,omitempty"`
}

// Run executes the simulation synchronously and returns the finished record.
func (s *Service) Run(ctx context.Context, userID uuid.UUID, in RunInput) (*Simulation, error) {
	brand, err := s.sender.GetByID(ctx, userID, in.BrandID)
	if err != nil {
		return nil, fmt.Errorf("brand not found: %w", err)
	}
	if in.LeadCount <= 0 || in.LeadCount > 25 {
		in.LeadCount = 8
	}
	if in.OnPositiveAction == "" {
		in.OnPositiveAction = domain.OnPositiveAutoDraftReply
	}
	if in.OnNegativeAction == "" {
		in.OnNegativeAction = domain.OnNegativeMarkCold
	}
	includeLessons := in.IncludeLessons == nil || *in.IncludeLessons
	personaKeys := in.Personas
	if len(personaKeys) == 0 {
		for _, p := range Personas {
			personaKeys = append(personaKeys, p.Key)
		}
	}

	leads, _, err := s.scoring.ListLeads(ctx, in.MarketID, userID, 0, 1, in.LeadCount)
	if err != nil {
		return nil, fmt.Errorf("load market leads: %w", err)
	}
	if len(leads) == 0 {
		return nil, fmt.Errorf("market has no leads to simulate")
	}

	pc, _ := json.Marshal(map[string]any{"personas": personaKeys, "lead_count": in.LeadCount})
	sim, err := s.repo.Create(ctx, Simulation{
		UserID: userID, Name: in.Name, MarketID: &in.MarketID,
		SenderProfileID: &in.BrandID, Steps: in.Steps, PersonaConfig: pc,
		Status: "running", Mode: "quick", Playbook: in.Playbook,
		OnPositiveAction: in.OnPositiveAction, OnNegativeAction: in.OnNegativeAction,
		IncludeLessons: includeLessons,
	})
	if err != nil {
		return nil, err
	}

	// Assign personas round-robin across the chosen leads.
	type job struct {
		biz     domain.BusinessWithRelevance
		persona Persona
	}
	var jobs []job
	for i := 0; i < len(leads); i++ {
		p, ok := personaByKey(personaKeys[i%len(personaKeys)])
		if !ok {
			continue
		}
		jobs = append(jobs, job{biz: leads[i], persona: p})
	}

	var (
		mu       sync.Mutex
		results  []Lead
		expected []string // expected sentiment per replied lead
		gotS     []string // classified sentiment per replied lead
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, j := range jobs {
		j := j
		g.Go(func() error {
			lead, sentExpected, sentGot := s.runLead(gctx, userID, brand, j.biz, j.persona, in.Steps, in.OnPositiveAction, in.OnNegativeAction, in.Playbook, includeLessons)
			lead.SimulationID = sim.ID
			if err := s.repo.AddLead(gctx, lead); err != nil {
				return err
			}
			mu.Lock()
			results = append(results, lead)
			if sentExpected != "" {
				expected = append(expected, sentExpected)
				gotS = append(gotS, sentGot)
			}
			count := len(results)
			mu.Unlock()
			s.broker.Publish(events.Event{
				Kind: events.KindSimulationProgress, UserID: userID,
				Data: map[string]any{"simulation_id": sim.ID.String(), "persona": j.persona.Key, "outcome": lead.Outcome, "done": count, "total": len(jobs)},
			})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		msg := err.Error()
		_ = s.repo.Finish(ctx, sim.ID, "failed", nil, &msg)
		return nil, err
	}

	summary := aggregate(results, expected, gotS)
	sumJSON, _ := json.Marshal(summary)
	if err := s.repo.Finish(ctx, sim.ID, "done", sumJSON, nil); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, userID, sim.ID)
}

func (s *Service) runLead(ctx context.Context, userID uuid.UUID, brand *domain.SenderProfile, biz domain.BusinessWithRelevance, persona Persona, steps []domain.SequenceStep, onPos, onNeg string, playbook map[string]string, includeLessons bool) (Lead, string, string) {
	lead := Lead{Persona: persona.Key, DisplayName: biz.Name, BusinessID: &biz.ID}

	if persona.NoEmail || biz.Email == nil || *biz.Email == "" {
		lead.Outcome = "skipped_no_email"
		lead.Transcript = []TranscriptMessage{
			{Who: "event", Day: 0, Body: "Skipped — no email address on file for this contact."},
		}
		return lead, "", ""
	}

	businessJSON := s.leadCtx.BusinessJSON(ctx, biz.ID)
	shipmentCtx := s.leadCtx.ShipmentContext(ctx, biz.ID)
	leadScoreJSON := s.leadCtx.LeadScoreJSON(ctx, userID, biz.ID)
	hasCatalog, catalogName := leadctx.SenderHasCatalog(brand)

	contact := domain.Contact{
		ID: uuid.New(), BusinessID: biz.ID, DisplayName: biz.Name, PrimaryEmail: biz.Email,
		PipelineStage: domain.PipelineLead,
	}
	contactJSON := prompts.EncodeJSON(contact)
	personaBiz := personaBusinessContext(biz)

	var transcript []TranscriptMessage
	base := time.Now().UTC()
	var lastDir *string
	lastAt := time.Time{}
	var lastClassified string
	repliedAtLeastOnce := false
	unsubscribed := false
	cold := false

	// --- touch 0: initial cold draft ---
	draftIn := prompts.OutreachDraftInput{
		SenderProfileJSON: prompts.EncodeJSON(brand),
		ContactJSON:       contactJSON,
		BusinessJSON:      businessJSON,
		ShipmentContext:   shipmentCtx,
		LeadScoreJSON:     leadScoreJSON,
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
	}
	subject, body := s.draftCold(ctx, userID, draftIn)
	body = prompts.WithSignature(body, brand.Signature)
	transcript = append(transcript, TranscriptMessage{Who: "you", Day: 0, Subject: subject, Body: body})
	out := domain.DirectionOutbound
	lastDir, lastAt = &out, base

	initialSubject := subject

	// handleReply mirrors production: the persona replies, we classify, then
	// branch (negative → on_negative, else → on_positive). Returns true once a
	// reply lands so the no-reply cadence stops. Closures capture the outer
	// state vars by reference.
	handleReply := func(day int, at time.Time, subj string) bool {
		rbody, intent := s.personaReply(ctx, userID, persona, personaBiz, transcript)
		// Production classifies only the stripped new text and runs the keyword
		// opt-out net on it — mirror both (persona bodies rarely quote, but the
		// path must be identical).
		newText := compliance.StripQuotedReply(rbody)
		sent, level, _, tags := s.classify(ctx, userID, subj, newText)
		if compliance.LooksLikeOptOut(newText) {
			tags = appendUnique(tags, domain.TagUnsubscribe)
		}
		// Keep the unsubscriber persona deterministic regardless of the classifier.
		if persona.unsubscribes || intent == "unsubscribing" {
			tags = appendUnique(tags, domain.TagUnsubscribe)
		}
		transcript = append(transcript, TranscriptMessage{Who: "them", Day: day, Body: rbody, Sentiment: sent, SentimentLevel: level, Tags: tags})
		in := domain.DirectionInbound
		lastDir, lastAt = &in, at
		// Production only marks the contact "Replied" for human replies — an
		// out-of-office bot doesn't count (mirrors poller's isBot gate).
		if !hasTag(tags, domain.TagOutOfOffice) {
			repliedAtLeastOnce = true
		}
		lastClassified = sent

		// Intent routing mirrors production (same pure fn): an intent tag can
		// override the sentiment branch.
	route:
		for {
			switch sequence.ResolveIntentRouting(tags) {
			case sequence.RouteSuppress:
				unsubscribed = true
				transcript = append(transcript, TranscriptMessage{Who: "event", Day: day, Body: "Recipient asked to unsubscribe — suppressed."})
				return true
			case sequence.RouteSnooze:
				// Out-of-office: production pauses the run for the snooze window,
				// deletes ONLY the out_of_office tag, then re-routes on the REMAINING
				// tags. Mirror that: log the pause, remove just the OOO tag, and
				// re-run the routing switch on what's left.
				transcript = append(transcript, TranscriptMessage{Who: "event", Day: day, Body: "Out-of-office auto-reply — paused 3 days, then resumed."})
				tags = removeTag(tags, domain.TagOutOfOffice)
				day += int(oooSnoozeWindowDays)
				continue route
			case sequence.RouteNotify:
				transcript = append(transcript, TranscriptMessage{Who: "event", Day: day, Body: "Wrong contact — notified you to find the right person."})
				return true
			}
			break route // sentiment branch — fall through
		}

		branch, action := sequence.ResolveReplyBranch(sent, onPos, onNeg)
		if branch == "negative" {
			if action == domain.OnNegativeNotify {
				transcript = append(transcript, TranscriptMessage{Who: "event", Day: day, Body: "Notified you to review the decline."})
			} else {
				cold = true
				transcript = append(transcript, TranscriptMessage{Who: "event", Day: day, Body: "Negative reply — marked cold, follow-ups stopped."})
			}
			return true
		}
		// positive / neutral → engage
		if action == domain.OnPositiveNotify {
			transcript = append(transcript, TranscriptMessage{Who: "event", Day: day, Body: "Notified you — warm lead, take it over."})
			return true
		}
		// auto_draft_reply: AI drafts a reply (for approval). Inject the same
		// standing guidance production would — the tested playbook + the brand's
		// remembered lessons matched to this reply's tags + sentiment.
		rb := s.draftFollowup(ctx, userID, prompts.OutreachReplyInput{
			Mode:              "reply",
			SenderProfileJSON: prompts.EncodeJSON(brand),
			ContactJSON:       contactJSON,
			BusinessJSON:      businessJSON,
			LeadScoreJSON:     leadScoreJSON,
			ConversationText:  renderThread(transcript),
			HasCatalog:        hasCatalog,
			CatalogFilename:   catalogName,
			Guidance:          s.simGuidance(ctx, userID, brand.ID, tags, sent, playbook, includeLessons),
		})
		transcript = append(transcript, TranscriptMessage{Who: "you", Day: day, Subject: "Re: " + strings.TrimPrefix(subject, "Re: "), Body: rb})
		return true
	}

	engaged := false
	if persona.RepliesAt(0) {
		engaged = handleReply(0, base, subject)
	}

	// --- no-reply cadence (runs only while silent) ---
	if !engaged {
		dayCursor := 0
		for i, step := range steps {
			if unsubscribed || cold {
				break
			}
			dayCursor += step.WaitDays
			now := base.Add(time.Duration(dayCursor) * 24 * time.Hour)
			if !sequence.EvaluateTriggerPure(step.Trigger, step.WaitDays, lastDir, lastAt, now, nil) {
				continue
			}
			switch step.Action {
			case domain.SequenceActionSendMessage:
				fbody := s.draftFollowup(ctx, userID, prompts.OutreachReplyInput{
					Mode:              "followup",
					SenderProfileJSON: prompts.EncodeJSON(brand),
					ContactJSON:       contactJSON,
					BusinessJSON:      businessJSON,
					LeadScoreJSON:     leadScoreJSON,
					ConversationText:  renderThread(transcript),
					HasCatalog:        hasCatalog,
					CatalogFilename:   catalogName,
				})
				subj := "Re: " + strings.TrimPrefix(initialSubject, "Re: ")
				if !step.AutoSend {
					// Production queues this followup as pending_approval unless the
					// step has auto_send AND a prior human-approved outbound exists.
					// The rehearsal can't wait on a human, so flag the pause and send.
					transcript = append(transcript, TranscriptMessage{Who: "event", Day: dayCursor, Body: "(queued for your approval — sent here so the rehearsal can continue)"})
				}
				transcript = append(transcript, TranscriptMessage{Who: "you", Day: dayCursor, Subject: subj, Body: fbody})
				lastDir, lastAt = &out, now
				if persona.RepliesAt(i+1) && handleReply(dayCursor, now, subj) {
					break
				}
			case domain.SequenceActionMarkCold:
				cold = true
				transcript = append(transcript, TranscriptMessage{Who: "event", Day: dayCursor, Body: "No reply after the cadence — marked cold."})
			}
		}
	}

	lead.Outcome = deriveOutcome(unsubscribed, cold, repliedAtLeastOnce, lastClassified)
	lead.Transcript = transcript

	// Judge the transcript.
	grade := s.judge(ctx, userID, persona, lead.Outcome, transcript)
	if grade != nil {
		lead.Grade = grade
	}

	return lead, persona.ExpectedSentiment, lastClassified
}

func deriveOutcome(unsub, cold, replied bool, lastSentiment string) string {
	switch {
	case unsub:
		return "unsubscribed"
	case cold:
		return "cold"
	case replied && lastSentiment == "positive":
		return "replied_positive"
	case replied && lastSentiment == "negative":
		return "replied_negative"
	case replied:
		return "replied"
	default:
		return "no_reply"
	}
}

func aggregate(leads []Lead, expected, got []string) Summary {
	sum := Summary{LeadCount: len(leads), Outcomes: map[string]int{}}
	var totScore, totHuman, graded float64
	for _, l := range leads {
		sum.Outcomes[l.Outcome]++
		if len(l.Grade) > 0 {
			var g prompts.JudgeResult
			if json.Unmarshal(l.Grade, &g) == nil {
				totScore += float64(g.Score)
				totHuman += float64(g.HumanFeel)
				graded++
			}
		}
	}
	if graded > 0 {
		sum.AvgScore = round1(totScore / graded)
		sum.AvgHumanFeel = round1(totHuman / graded)
	}
	if len(expected) > 0 {
		match := 0
		for i := range expected {
			if i < len(got) && expected[i] == got[i] {
				match++
			}
		}
		acc := round1(100 * float64(match) / float64(len(expected)))
		sum.SentimentAccuracy = &acc
	}
	return sum
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// oooSnoozeWindowDays mirrors the engine's oooSnoozeWindow (3 days) so the Lab's
// out-of-office resume timing matches production.
const oooSnoozeWindowDays = 3

// simLessonLimit mirrors replyguidance's lessonLimit: only the 6 most recent
// matching lessons are injected, ordered oldest→newest (newest lands last —
// most salient position, it wins on conflicts).
const simLessonLimit = 6

// simGuidance mirrors production's reply guidance (replyguidance.Load) for the
// Lab: the transient playbook (tag → instruction) plus the brand's remembered
// lessons, matched to this reply's tags + sentiment bucket. Uses the sender
// repo (no live convo). includeLessons=false skips the brand lessons entirely.
func (s *Service) simGuidance(ctx context.Context, userID, brandID uuid.UUID, tags []string, bucket string, playbook map[string]string, includeLessons bool) string {
	var lines []string
	for _, t := range tags {
		if ins, ok := playbook[t]; ok && strings.TrimSpace(ins) != "" {
			lines = append(lines, "- "+replyguidance.SanitizeLine(ins))
		}
	}
	if includeLessons {
		lessons, _ := s.sender.ListLessons(ctx, userID, brandID)
		var matched []sender.Lesson
		for _, l := range lessons {
			if l.MatchSentiment != bucket {
				continue
			}
			if len(l.MatchTags) > 0 && !overlaps(l.MatchTags, tags) {
				continue
			}
			matched = append(matched, l)
		}
		// Take the 6 MOST RECENT matches, then inject oldest→newest.
		sort.Slice(matched, func(i, j int) bool { return matched[i].CreatedAt.After(matched[j].CreatedAt) })
		if len(matched) > simLessonLimit {
			matched = matched[:simLessonLimit]
		}
		for i := len(matched) - 1; i >= 0; i-- {
			lines = append(lines, "- "+replyguidance.SanitizeLine(matched[i].Instruction))
		}
	}
	return strings.Join(lines, "\n")
}

// removeTag returns a copy of tags without the given tag. A copy (not an
// in-place filter) because the original slice is already referenced by a
// transcript message.
func removeTag(tags []string, tag string) []string {
	var out []string
	for _, t := range tags {
		if t != tag {
			out = append(out, t)
		}
	}
	return out
}

func overlaps(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func hasTag(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

func personaBusinessContext(biz domain.BusinessWithRelevance) string {
	parts := []string{biz.Name}
	if biz.CountryCode != nil {
		parts = append(parts, "in "+*biz.CountryCode)
	}
	if biz.Industry != nil && *biz.Industry != "" {
		parts = append(parts, *biz.Industry)
	}
	if biz.BusinessType != nil && *biz.BusinessType != "" {
		parts = append(parts, *biz.BusinessType)
	}
	return strings.Join(parts, " · ")
}

func renderThread(t []TranscriptMessage) string {
	var b strings.Builder
	for _, m := range t {
		who := "YOU"
		if m.Who == "them" {
			who = "THEM"
		} else if m.Who == "event" {
			continue
		}
		subj := m.Subject
		if subj == "" {
			subj = "(no subject)"
		}
		b.WriteString(fmt.Sprintf("--- %s (day %d) ---\nSubject: %s\n%s\n\n", who, m.Day, subj, m.Body))
	}
	return b.String()
}
