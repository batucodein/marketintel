package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// ConvDraftRefiner rewrites a single conversation's pending reply draft via the
// real reply prompt + guard. Bound to conversation.Service in main.go (campaign
// already imports conversation, so no import cycle). remember is always false
// here — the playbook is the explicit memory, not auto-learned lessons.
type ConvDraftRefiner interface {
	RefineDraft(ctx context.Context, userID, conversationID, draftID uuid.UUID, instruction, previousBody string, remember bool) (*domain.Message, error)
}

// SetConvRefiner wires the conversation reply-draft refiner after construction.
func (s *Service) SetConvRefiner(r ConvDraftRefiner) { s.convRefiner = r }

// rewriteColdDrafts rewrites every current cold draft (status='drafted') to
// follow the instruction. One-time edit — nothing is remembered.
func (s *Service) rewriteColdDrafts(ctx context.Context, userID, campaignID uuid.UUID, instruction string, progress func(done, total int)) (int, int, error) {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return 0, 0, err
	}
	sp, _ := s.resolveBrand(ctx, camp)
	if sp == nil {
		sp = &domain.SenderProfile{UserID: camp.UserID, Tone: "formal"}
	}
	if o := s.parsePositioning(camp); o != nil {
		if o.ProductDescription != "" {
			sp.ProductDescription = o.ProductDescription
		}
		if o.ValueProp != "" {
			sp.ValueProp = o.ValueProp
		}
		if o.Tone != "" {
			sp.Tone = o.Tone
		}
	}
	positioning := camp.Goal
	if o := s.parsePositioning(camp); o != nil && o.Goal != "" {
		positioning = o.Goal
	}
	hasCatalog := camp.AttachCatalog
	catalogName := ""
	if hasCatalog {
		hasCatalog, catalogName = senderCatalogIfPresent(sp)
	}

	type target struct{ contactID, draftID uuid.UUID }
	rows, err := s.pool.Query(ctx,
		`SELECT contact_id, draft_message_id FROM campaign_contacts
		  WHERE campaign_id=$1 AND status='drafted' AND draft_message_id IS NOT NULL`, campaignID)
	if err != nil {
		return 0, 0, err
	}
	var targets []target
	for rows.Next() {
		var t target
		if rows.Scan(&t.contactID, &t.draftID) == nil {
			targets = append(targets, t)
		}
	}
	rows.Close()
	total := len(targets)
	if total == 0 {
		return 0, 0, nil
	}

	var (
		mu      sync.Mutex
		updated int
		failed  int
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, t := range targets {
		t := t
		g.Go(func() error {
			err := s.refineColdDraft(gctx, *camp, sp, positioning, hasCatalog, catalogName, instruction, t.contactID, t.draftID)
			mu.Lock()
			if err != nil {
				failed++
				slog.Warn("agent: cold draft rewrite failed", "campaign_id", campaignID, "contact_id", t.contactID, "error", err)
			} else {
				updated++
			}
			done := updated + failed
			mu.Unlock()
			if progress != nil {
				progress(done, total)
			}
			return nil
		})
	}
	_ = g.Wait()
	return updated, failed, nil
}

func (s *Service) refineColdDraft(ctx context.Context, camp domain.Campaign, sp *domain.SenderProfile, positioning string, hasCatalog bool, catalogName, instruction string, contactID, draftID uuid.UUID) error {
	var subject, body string
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(subject,''), COALESCE(body_text,'') FROM messages
		  WHERE id=$1 AND status IN ('draft','pending_approval')`, draftID,
	).Scan(&subject, &body); err != nil {
		return fmt.Errorf("load draft: %w", err)
	}
	c, err := s.contacts.Get(ctx, camp.UserID, contactID)
	if err != nil {
		return fmt.Errorf("load contact: %w", err)
	}
	in := prompts.OutreachDraftInput{
		SenderProfileJSON:   prompts.EncodeJSON(sp),
		CampaignPositioning: positioning,
		ContactJSON:         prompts.EncodeJSON(c),
		BusinessJSON:        s.leadCtx.BusinessJSON(ctx, c.BusinessID),
		ShipmentContext:     s.leadCtx.ShipmentContext(ctx, c.BusinessID),
		LeadScoreJSON:       s.leadCtx.LeadScoreJSON(ctx, camp.UserID, c.BusinessID),
		HasCatalog:          hasCatalog,
		CatalogFilename:     catalogName,
		RefineInstruction:   instruction,
		PreviousDraft:       body,
	}
	once := func() (*prompts.OutreachDraftResult, error) {
		p := prompts.BuildOutreachDraftPrompt(in)
		raw, _, aerr := s.ai.CompleteJSON(ai.WithUserID(ctx, camp.UserID), "outreach_draft", p.Prompt, p.System, 0)
		if aerr != nil {
			return nil, aerr
		}
		var out prompts.OutreachDraftResult
		if jerr := json.Unmarshal(raw, &out); jerr != nil {
			return nil, jerr
		}
		if out.Body == "" {
			return nil, errors.New("ai returned empty body")
		}
		return &out, nil
	}
	out, err := once()
	if err != nil {
		return err
	}
	if len(prompts.ValidateOutreachBody(out.Body, hasCatalog, prompts.DraftModeCold)) > 0 {
		in.StricterRetry = true
		if second, rerr := once(); rerr == nil && second != nil {
			out = second
		}
	}
	out.Body = prompts.WithSignature(out.Body, sp.Signature)
	if out.Subject == "" {
		out.Subject = subject
	}
	_, err = s.pool.Exec(ctx,
		`UPDATE messages SET subject=$1, body_text=$2, body_html=NULL
		  WHERE id=$3 AND status IN ('draft','pending_approval')`,
		out.Subject, out.Body, draftID)
	return err
}

// rewriteReplyDrafts rewrites the group's pending reply drafts (created by the
// sequence engine on inbound replies) to follow the instruction. sentiment is
// "" (all reply drafts) or positive|neutral|negative to scope a cohort. Reuses
// the conversation reply prompt + guard via the injected refiner.
func (s *Service) rewriteReplyDrafts(ctx context.Context, userID, campaignID uuid.UUID, instruction, sentiment string, progress func(done, total int)) (int, int, error) {
	if s.convRefiner == nil {
		return 0, 0, errors.New("reply-draft editing not available")
	}
	q := `SELECT cv.id, pm.id, COALESCE(pm.body_text,'')
	        FROM conversations cv
	        JOIN messages pm ON pm.conversation_id = cv.id
	         AND pm.direction='out' AND pm.status='pending_approval'
	       WHERE cv.campaign_id=$1`
	args := []any{campaignID}
	if sentiment != "" {
		q += ` AND cv.last_inbound_sentiment=$2`
		args = append(args, sentiment)
	}
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return 0, 0, err
	}
	type target struct {
		convID, draftID uuid.UUID
		body            string
	}
	var targets []target
	for rows.Next() {
		var t target
		if rows.Scan(&t.convID, &t.draftID, &t.body) == nil {
			targets = append(targets, t)
		}
	}
	rows.Close()
	total := len(targets)
	if total == 0 {
		return 0, 0, nil
	}

	var (
		mu      sync.Mutex
		updated int
		failed  int
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(4)
	for _, t := range targets {
		t := t
		g.Go(func() error {
			_, rerr := s.convRefiner.RefineDraft(gctx, userID, t.convID, t.draftID, instruction, t.body, false)
			mu.Lock()
			if rerr != nil {
				failed++
				slog.Warn("agent: reply draft rewrite failed", "conversation_id", t.convID, "error", rerr)
			} else {
				updated++
			}
			done := updated + failed
			mu.Unlock()
			if progress != nil {
				progress(done, total)
			}
			return nil
		})
	}
	_ = g.Wait()
	return updated, failed, nil
}
