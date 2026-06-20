package simulation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/compliance"
	"github.com/batuhan/marketintel/internal/outreach/events"
	"github.com/batuhan/marketintel/internal/outreach/leadctx"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
	"github.com/batuhan/marketintel/internal/outreach/sequence"
)

// engine.go is the interactive, step-by-step simulation: the same outreach
// sequence runLead walks, but decomposed into a resumable, approval-gated state
// machine the user can advance one cadence touch at a time and intervene in.
// It reuses the exact AI-call helpers (draftCold/draftFollowup/classify/
// personaReply/judge/simGuidance) and the shared pure decision functions
// (EvaluateTriggerPure/ResolveIntentRouting/ResolveReplyBranch), so every
// decision matches production.

// Lead states.
const (
	stateActive    = "active"            // silent; advances along the cadence
	stateAwaiting  = "awaiting_approval" // a draft is pending the user
	stateSnoozed   = "snoozed"           // out-of-office; resumes after the window
	stateDone      = "done"              // terminal
)

// simEpoch anchors the virtual day clock so EvaluateTriggerPure (which works in
// time.Time) sees the same "wait_days elapsed" decision the engine does.
var simEpoch = time.Unix(0, 0).UTC()

func dayToTime(d int) time.Time { return simEpoch.Add(time.Duration(d) * 24 * time.Hour) }

// CreateInteractive starts a stepped simulation: drafts every lead's cold opener
// up front (so the user reviews a batch of pending cold drafts, exactly like a
// live group), then pauses awaiting approval.
func (s *Service) CreateInteractive(ctx context.Context, userID uuid.UUID, in RunInput) (*Simulation, error) {
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
		UserID: userID, Name: in.Name, MarketID: &in.MarketID, SenderProfileID: &in.BrandID,
		Steps: in.Steps, PersonaConfig: pc, Status: "paused", Mode: "interactive",
		Playbook: in.Playbook, OnPositiveAction: in.OnPositiveAction,
		OnNegativeAction: in.OnNegativeAction, IncludeLessons: includeLessons,
	})
	if err != nil {
		return nil, err
	}

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

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	var mu sync.Mutex
	done := 0
	for _, j := range jobs {
		j := j
		g.Go(func() error {
			lead := s.buildInitialLead(gctx, userID, brand, j.biz, j.persona, sim.ID)
			if err := s.repo.InsertLead(gctx, lead); err != nil {
				return err
			}
			mu.Lock()
			done++
			d := done
			mu.Unlock()
			s.broker.Publish(events.Event{
				Kind: events.KindSimulationProgress, UserID: userID,
				Data: map[string]any{"simulation_id": sim.ID.String(), "persona": j.persona.Key, "done": d, "total": len(jobs)},
			})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		msg := err.Error()
		_ = s.repo.Finish(ctx, sim.ID, "failed", nil, &msg)
		return nil, err
	}
	return s.repo.Get(ctx, userID, sim.ID)
}

// buildInitialLead drafts the cold opener and returns a lead awaiting approval
// (or a finished skipped lead when there's no email).
func (s *Service) buildInitialLead(ctx context.Context, userID uuid.UUID, brand *domain.SenderProfile, biz domain.BusinessWithRelevance, persona Persona, simID uuid.UUID) Lead {
	lead := Lead{ID: uuid.New(), SimulationID: simID, Persona: persona.Key, DisplayName: biz.Name, BusinessID: &biz.ID, State: stateActive}

	if persona.NoEmail || biz.Email == nil || *biz.Email == "" {
		lead.State = stateDone
		lead.Outcome = "skipped_no_email"
		lead.Transcript = []TranscriptMessage{{Who: "event", Day: 0, Body: "Skipped — no email address on file for this contact."}}
		return lead
	}

	hasCatalog, catalogName := leadctx.SenderHasCatalog(brand)
	contact := domain.Contact{ID: uuid.New(), BusinessID: biz.ID, DisplayName: biz.Name, PrimaryEmail: biz.Email, PipelineStage: domain.PipelineLead}
	pctx := LeadPromptCtx{
		BusinessJSON:  s.leadCtx.BusinessJSON(ctx, biz.ID),
		ShipmentCtx:   s.leadCtx.ShipmentContext(ctx, biz.ID),
		LeadScoreJSON: s.leadCtx.LeadScoreJSON(ctx, userID, biz.ID),
		ContactJSON:   prompts.EncodeJSON(contact),
		PersonaBiz:    personaBusinessContext(biz),
		HasCatalog:    hasCatalog,
		CatalogName:   catalogName,
	}

	subject, body := s.draftCold(ctx, userID, prompts.OutreachDraftInput{
		SenderProfileJSON: prompts.EncodeJSON(brand),
		ContactJSON:       pctx.ContactJSON,
		BusinessJSON:      pctx.BusinessJSON,
		ShipmentContext:   pctx.ShipmentCtx,
		LeadScoreJSON:     pctx.LeadScoreJSON,
		HasCatalog:        hasCatalog,
		CatalogFilename:   catalogName,
	})
	body = prompts.WithSignature(body, brand.Signature)
	pctx.InitialSubj = subject

	lead.PromptCtx = &pctx
	lead.PendingDraft = &PendingDraft{Kind: "cold", Subject: subject, Body: body, Step: -1}
	lead.State = stateAwaiting
	lead.Touch = 0
	return lead
}

// Advance moves the virtual clock to the next due touch and steps every
// actionable lead until it pauses (awaiting approval), snoozes, or finishes.
func (s *Service) Advance(ctx context.Context, userID, simID uuid.UUID) (*Simulation, error) {
	sim, err := s.repo.Get(ctx, userID, simID)
	if err != nil {
		return nil, err
	}
	brand, err := s.sender.GetByID(ctx, userID, *sim.SenderProfileID)
	if err != nil {
		return nil, fmt.Errorf("brand not found: %w", err)
	}

	// Pick the next day: the soonest due active lead or snooze expiry.
	next := sim.VirtualDay
	have := false
	consider := func(d int) {
		if d < sim.VirtualDay {
			d = sim.VirtualDay
		}
		if !have || d < next {
			next, have = d, true
		}
	}
	for i := range sim.Leads {
		l := sim.Leads[i]
		if l.State == stateActive && l.CurrentStep < len(sim.Steps) {
			consider(l.NextDay)
		} else if l.State == stateSnoozed && l.SnoozeUntilDay != nil {
			consider(*l.SnoozeUntilDay)
		}
	}
	if have {
		sim.VirtualDay = next
	}

	for i := range sim.Leads {
		lead := &sim.Leads[i]
		switch {
		case lead.State == stateSnoozed && lead.SnoozeUntilDay != nil && *lead.SnoozeUntilDay <= sim.VirtualDay:
			persona, _ := personaByKey(lead.Persona)
			tags := lead.ResumeTags
			sent := ""
			if lead.ResumeSentiment != nil {
				sent = *lead.ResumeSentiment
			}
			lead.SnoozeUntilDay = nil
			lead.ResumeTags = nil
			lead.ResumeSentiment = nil
			lead.State = stateActive
			s.applyRouting(ctx, userID, sim, brand, lead, persona, sim.VirtualDay, tags, sent)
			if err := s.repo.UpdateLead(ctx, *lead); err != nil {
				return nil, err
			}
		case lead.State == stateActive:
			if s.stepLead(ctx, userID, sim, brand, lead) {
				if err := s.repo.UpdateLead(ctx, *lead); err != nil {
					return nil, err
				}
			}
		}
	}

	status, summary := s.summarize(sim)
	if err := s.repo.UpdateSim(ctx, simID, sim.VirtualDay, status, summary); err != nil {
		return nil, err
	}
	s.broker.Publish(events.Event{Kind: events.KindSimulationProgress, UserID: userID,
		Data: map[string]any{"simulation_id": simID.String(), "action": "advanced", "virtual_day": sim.VirtualDay}})
	return s.repo.Get(ctx, userID, simID)
}

// stepLead advances one active, silent lead by one cadence step at the sim's
// current virtual day. Returns true when it changed the lead.
func (s *Service) stepLead(ctx context.Context, userID uuid.UUID, sim *Simulation, brand *domain.SenderProfile, lead *Lead) bool {
	if lead.State != stateActive {
		return false
	}
	persona, _ := personaByKey(lead.Persona)
	if lead.CurrentStep >= len(sim.Steps) {
		s.finalizeLead(ctx, userID, lead, persona, "no_reply")
		return true
	}
	step := sim.Steps[lead.CurrentStep]
	out := domain.DirectionOutbound
	if !sequence.EvaluateTriggerPure(step.Trigger, step.WaitDays, &out, dayToTime(lead.LastOutDay), dayToTime(sim.VirtualDay), nil) {
		return false // not due yet
	}
	switch step.Action {
	case domain.SequenceActionSendMessage:
		pctx := lead.PromptCtx
		fbody := s.draftFollowup(ctx, userID, prompts.OutreachReplyInput{
			Mode:              "followup",
			SenderProfileJSON: prompts.EncodeJSON(brand),
			ContactJSON:       pctx.ContactJSON,
			BusinessJSON:      pctx.BusinessJSON,
			LeadScoreJSON:     pctx.LeadScoreJSON,
			ConversationText:  renderThread(lead.Transcript),
			HasCatalog:        pctx.HasCatalog,
			CatalogFilename:   pctx.CatalogName,
		})
		subj := "Re: " + strings.TrimPrefix(pctx.InitialSubj, "Re: ")
		lead.PendingDraft = &PendingDraft{Kind: "followup", Subject: subj, Body: fbody, Step: lead.CurrentStep}
		lead.Touch = lead.CurrentStep + 1
		lead.State = stateAwaiting
		return true
	case domain.SequenceActionMarkCold:
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: sim.VirtualDay, Body: "No reply after the cadence — marked cold."})
		s.finalizeLead(ctx, userID, lead, persona, "cold")
		return true
	case domain.SequenceActionNotifyUser:
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: sim.VirtualDay, Body: "Cadence step notified you to take a look."})
		s.advanceCursor(lead, sim.Steps)
		return true
	case domain.SequenceActionAdvanceStage:
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: sim.VirtualDay, Body: "Cadence step advanced the pipeline stage."})
		s.advanceCursor(lead, sim.Steps)
		return true
	default:
		s.advanceCursor(lead, sim.Steps)
		return true
	}
}

// advanceCursor moves to the next cadence step and accumulates its wait into the
// virtual-day cursor (mirrors runLead's dayCursor += step.WaitDays).
func (s *Service) advanceCursor(lead *Lead, steps []domain.SequenceStep) {
	lead.CurrentStep++
	if lead.CurrentStep < len(steps) {
		lead.NextDay += steps[lead.CurrentStep].WaitDays
	}
}

// scheduleCold schedules the first cadence step relative to the cold send.
func (s *Service) scheduleCold(lead *Lead, steps []domain.SequenceStep) {
	if len(steps) > 0 {
		lead.NextDay = lead.LastOutDay + steps[0].WaitDays
	}
}

// ApproveLead sends the lead's pending draft (virtually), then continues the
// production path: the persona may reply → classify → route, or the cadence
// schedules the next step.
func (s *Service) ApproveLead(ctx context.Context, userID, simID, leadID uuid.UUID) (*Simulation, error) {
	sim, brand, lead, err := s.loadForLead(ctx, userID, simID, leadID)
	if err != nil {
		return nil, err
	}
	if lead.State != stateAwaiting || lead.PendingDraft == nil {
		return nil, fmt.Errorf("no draft awaiting approval for this lead")
	}
	persona, _ := personaByKey(lead.Persona)
	day := sim.VirtualDay
	pd := lead.PendingDraft

	lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "you", Day: day, Subject: pd.Subject, Body: pd.Body})
	out := domain.DirectionOutbound
	lead.LastDirection = &out
	lead.LastOutDay = day
	kind := pd.Kind
	lead.PendingDraft = nil
	lead.State = stateActive

	switch kind {
	case "reply":
		// Approving the auto-drafted reply is the end of engagement (runLead
		// returns true after drafting the reply).
		s.finalizeLead(ctx, userID, lead, persona, "")
	default: // cold | followup
		if persona.RepliesAt(lead.Touch) {
			s.routeReply(ctx, userID, sim, brand, lead, persona, day, pd.Subject)
		} else if kind == "cold" {
			s.scheduleCold(lead, sim.Steps)
			if len(sim.Steps) == 0 {
				s.finalizeLead(ctx, userID, lead, persona, "no_reply")
			}
		} else { // followup, no reply → next step
			s.advanceCursor(lead, sim.Steps)
		}
	}

	if err := s.repo.UpdateLead(ctx, *lead); err != nil {
		return nil, err
	}
	return s.afterLeadChange(ctx, userID, sim)
}

// routeReply mirrors runLead's handleReply: persona replies, we classify, then
// intent/sentiment routing decides the next state.
func (s *Service) routeReply(ctx context.Context, userID uuid.UUID, sim *Simulation, brand *domain.SenderProfile, lead *Lead, persona Persona, day int, subj string) {
	pctx := lead.PromptCtx
	rbody, intent := s.personaReply(ctx, userID, persona, pctx.PersonaBiz, lead.Transcript)
	newText := compliance.StripQuotedReply(rbody)
	sent, level, _, tags := s.classify(ctx, userID, subj, newText)
	if compliance.LooksLikeOptOut(newText) {
		tags = appendUnique(tags, domain.TagUnsubscribe)
	}
	if persona.unsubscribes || intent == "unsubscribing" {
		tags = appendUnique(tags, domain.TagUnsubscribe)
	}
	lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "them", Day: day, Body: rbody, Sentiment: sent, SentimentLevel: level, Tags: tags})
	in := domain.DirectionInbound
	lead.LastDirection = &in
	if !hasTag(tags, domain.TagOutOfOffice) {
		lead.RepliedOnce = true
	}
	lead.LastSentiment = &sent
	s.applyRouting(ctx, userID, sim, brand, lead, persona, day, tags, sent)
}

// applyRouting runs the intent precedence + sentiment branch (the same pure
// functions production uses) and sets the lead's resulting state. Out-of-office
// SNOOZES the lead (stepped) and stashes the remaining tags to re-route on resume.
func (s *Service) applyRouting(ctx context.Context, userID uuid.UUID, sim *Simulation, brand *domain.SenderProfile, lead *Lead, persona Persona, day int, tags []string, sent string) {
	switch sequence.ResolveIntentRouting(tags) {
	case sequence.RouteSuppress:
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: day, Body: "Recipient asked to unsubscribe — suppressed."})
		s.finalizeLead(ctx, userID, lead, persona, "unsubscribed")
		return
	case sequence.RouteSnooze:
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: day, Body: "Out-of-office auto-reply — snoozed 3 days."})
		remaining := removeTag(tags, domain.TagOutOfOffice)
		until := day + oooSnoozeWindowDays
		lead.State = stateSnoozed
		lead.SnoozeUntilDay = &until
		lead.ResumeTags = remaining
		lead.ResumeSentiment = &sent
		return
	case sequence.RouteNotify:
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: day, Body: "Wrong contact — notified you to find the right person."})
		s.finalizeLead(ctx, userID, lead, persona, "")
		return
	}

	branch, action := sequence.ResolveReplyBranch(sent, sim.OnPositiveAction, sim.OnNegativeAction)
	if branch == "negative" {
		if action == domain.OnNegativeNotify {
			lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: day, Body: "Notified you to review the decline."})
			s.finalizeLead(ctx, userID, lead, persona, "")
		} else {
			lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: day, Body: "Negative reply — marked cold, follow-ups stopped."})
			s.finalizeLead(ctx, userID, lead, persona, "cold")
		}
		return
	}
	if action == domain.OnPositiveNotify {
		lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: day, Body: "Notified you — warm lead, take it over."})
		s.finalizeLead(ctx, userID, lead, persona, "")
		return
	}
	// auto_draft_reply → draft a reply for approval (with the same playbook +
	// lessons guidance production injects).
	pctx := lead.PromptCtx
	rb := s.draftFollowup(ctx, userID, prompts.OutreachReplyInput{
		Mode:              "reply",
		SenderProfileJSON: prompts.EncodeJSON(brand),
		ContactJSON:       pctx.ContactJSON,
		BusinessJSON:      pctx.BusinessJSON,
		LeadScoreJSON:     pctx.LeadScoreJSON,
		ConversationText:  renderThread(lead.Transcript),
		HasCatalog:        pctx.HasCatalog,
		CatalogFilename:   pctx.CatalogName,
		Guidance:          s.simGuidance(ctx, userID, brand.ID, tags, sent, sim.Playbook, sim.IncludeLessons),
	})
	lead.PendingDraft = &PendingDraft{Kind: "reply", Subject: "Re: " + strings.TrimPrefix(pctx.InitialSubj, "Re: "), Body: rb, Step: -1}
	lead.State = stateAwaiting
}

// EditLead replaces the pending draft's subject/body with the user's edit.
func (s *Service) EditLead(ctx context.Context, userID, simID, leadID uuid.UUID, subject, body string) (*Simulation, error) {
	sim, _, lead, err := s.loadForLead(ctx, userID, simID, leadID)
	if err != nil {
		return nil, err
	}
	if lead.State != stateAwaiting || lead.PendingDraft == nil {
		return nil, fmt.Errorf("no draft to edit")
	}
	if strings.TrimSpace(subject) != "" {
		lead.PendingDraft.Subject = subject
	}
	lead.PendingDraft.Body = body
	if err := s.repo.UpdateLead(ctx, *lead); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, userID, sim.ID)
}

// RefineLead re-runs the draft prompt with an instruction (reusing the same path
// runLead uses), replacing the pending draft.
func (s *Service) RefineLead(ctx context.Context, userID, simID, leadID uuid.UUID, instruction string) (*Simulation, error) {
	sim, brand, lead, err := s.loadForLead(ctx, userID, simID, leadID)
	if err != nil {
		return nil, err
	}
	if lead.State != stateAwaiting || lead.PendingDraft == nil {
		return nil, fmt.Errorf("no draft to refine")
	}
	s.refinePending(ctx, userID, brand, lead, instruction)
	if err := s.repo.UpdateLead(ctx, *lead); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, userID, sim.ID)
}

// refinePending rewrites a lead's pending draft in place with the instruction,
// reusing the same draft/reply prompt path the rest of the engine uses.
func (s *Service) refinePending(ctx context.Context, userID uuid.UUID, brand *domain.SenderProfile, lead *Lead, instruction string) {
	pctx := lead.PromptCtx
	pd := lead.PendingDraft
	if pd == nil || pctx == nil {
		return
	}
	if pd.Kind == "cold" {
		subject, body := s.draftCold(ctx, userID, prompts.OutreachDraftInput{
			SenderProfileJSON: prompts.EncodeJSON(brand),
			ContactJSON:       pctx.ContactJSON,
			BusinessJSON:      pctx.BusinessJSON,
			ShipmentContext:   pctx.ShipmentCtx,
			LeadScoreJSON:     pctx.LeadScoreJSON,
			HasCatalog:        pctx.HasCatalog,
			CatalogFilename:   pctx.CatalogName,
			RefineInstruction: instruction,
			PreviousDraft:     pd.Body,
		})
		pd.Body = prompts.WithSignature(body, brand.Signature)
		if strings.TrimSpace(subject) != "" {
			pd.Subject = subject
		}
		return
	}
	mode := "followup"
	if pd.Kind == "reply" {
		mode = "reply"
	}
	pd.Body = s.draftFollowup(ctx, userID, prompts.OutreachReplyInput{
		Mode:              mode,
		SenderProfileJSON: prompts.EncodeJSON(brand),
		ContactJSON:       pctx.ContactJSON,
		BusinessJSON:      pctx.BusinessJSON,
		LeadScoreJSON:     pctx.LeadScoreJSON,
		ConversationText:  renderThread(lead.Transcript),
		HasCatalog:        pctx.HasCatalog,
		CatalogFilename:   pctx.CatalogName,
		RefineInstruction: instruction,
		PreviousDraft:     pd.Body,
	})
}

// DismissLead drops the pending draft and ends that lead (manual stop).
func (s *Service) DismissLead(ctx context.Context, userID, simID, leadID uuid.UUID) (*Simulation, error) {
	sim, _, lead, err := s.loadForLead(ctx, userID, simID, leadID)
	if err != nil {
		return nil, err
	}
	if lead.State != stateAwaiting {
		return nil, fmt.Errorf("nothing to dismiss")
	}
	persona, _ := personaByKey(lead.Persona)
	lead.Transcript = append(lead.Transcript, TranscriptMessage{Who: "event", Day: sim.VirtualDay, Body: "You dismissed the draft — lead stopped."})
	lead.PendingDraft = nil
	s.finalizeLead(ctx, userID, lead, persona, "no_reply")
	if err := s.repo.UpdateLead(ctx, *lead); err != nil {
		return nil, err
	}
	return s.afterLeadChange(ctx, userID, sim)
}

// SetPlaybook persists an edit to the sim's reply playbook (used mid-run).
func (s *Service) SetPlaybook(ctx context.Context, userID, simID uuid.UUID, pb map[string]string) (*Simulation, error) {
	if _, err := s.repo.Get(ctx, userID, simID); err != nil {
		return nil, err
	}
	if err := s.repo.SetPlaybook(ctx, simID, pb); err != nil {
		return nil, err
	}
	return s.repo.Get(ctx, userID, simID)
}

// finalizeLead marks a lead done, computes its outcome (explicit or derived),
// clears any pending draft, and judges the transcript.
func (s *Service) finalizeLead(ctx context.Context, userID uuid.UUID, lead *Lead, persona Persona, explicit string) {
	lead.State = stateDone
	lead.PendingDraft = nil
	if explicit != "" {
		lead.Outcome = explicit
	} else {
		last := ""
		if lead.LastSentiment != nil {
			last = *lead.LastSentiment
		}
		lead.Outcome = deriveOutcome(false, false, lead.RepliedOnce, last)
	}
	if grade := s.judge(ctx, userID, persona, lead.Outcome, lead.Transcript); grade != nil {
		lead.Grade = grade
	}
}

// loadForLead fetches the sim, its brand, and one lead in one place.
func (s *Service) loadForLead(ctx context.Context, userID, simID, leadID uuid.UUID) (*Simulation, *domain.SenderProfile, *Lead, error) {
	sim, err := s.repo.Get(ctx, userID, simID)
	if err != nil {
		return nil, nil, nil, err
	}
	brand, err := s.sender.GetByID(ctx, userID, *sim.SenderProfileID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("brand not found: %w", err)
	}
	lead, err := s.repo.GetLead(ctx, simID, leadID)
	if err != nil {
		return nil, nil, nil, err
	}
	return sim, brand, lead, nil
}

// afterLeadChange recomputes the run summary/status and returns the fresh sim.
func (s *Service) afterLeadChange(ctx context.Context, userID uuid.UUID, sim *Simulation) (*Simulation, error) {
	fresh, err := s.repo.Get(ctx, userID, sim.ID)
	if err != nil {
		return nil, err
	}
	status, summary := s.summarize(fresh)
	if err := s.repo.UpdateSim(ctx, sim.ID, fresh.VirtualDay, status, summary); err != nil {
		return nil, err
	}
	fresh.Status = status
	fresh.Summary = summary
	return fresh, nil
}

// summarize derives the run status (done when every lead is terminal, else
// paused) and the outcome/score summary.
func (s *Service) summarize(sim *Simulation) (string, json.RawMessage) {
	allDone := true
	var expected, got []string
	for _, l := range sim.Leads {
		if l.State != stateDone {
			allDone = false
		}
		persona, _ := personaByKey(l.Persona)
		if persona.ExpectedSentiment != "" && l.LastSentiment != nil {
			expected = append(expected, persona.ExpectedSentiment)
			got = append(got, *l.LastSentiment)
		}
	}
	status := "paused"
	if allDone {
		status = "done"
	}
	summary := aggregate(sim.Leads, expected, got)
	b, _ := json.Marshal(summary)
	return status, b
}
