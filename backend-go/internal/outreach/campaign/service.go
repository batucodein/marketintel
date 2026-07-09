package campaign

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/outreach/contact"
	"github.com/batuhan/marketintel/internal/outreach/conversation"
	"github.com/batuhan/marketintel/internal/outreach/leadctx"
	"github.com/batuhan/marketintel/internal/outreach/sender"
	"github.com/batuhan/marketintel/internal/platform/ai"
)

// SequenceStarter starts a sequence run for a (sequence, conversation) pair.
// Defined as an interface here to avoid an import cycle between the campaign
// and sequence packages — the wiring layer in main.go binds it to a thin
// adapter around sequence.Service.
type SequenceStarter interface {
	StartCampaignRun(ctx context.Context, sequenceID, conversationID uuid.UUID) error
}

// Service is the orchestration facade for campaign-related business logic.
// It wires the campaign repo together with everything the drafter / scheduler
// workers need (channel registry, AI router, contact + sender repos, lead
// context loader).
type Service struct {
	repo         Repository
	channels     channel.Repository
	contacts     contact.Repository
	sender       sender.Repository
	conversation conversation.Repository
	registry     *channel.Registry
	ai           *ai.Router
	leadCtx      *leadctx.Loader
	pool         *pgxpool.Pool
	publicAPIURL string
	sequenceStarter SequenceStarter
	convRefiner     ConvDraftRefiner
}

// SetSequenceStarter is called from main.go after the sequence service is
// constructed (otherwise wiring would be a chicken-and-egg).
func (s *Service) SetSequenceStarter(st SequenceStarter) {
	s.sequenceStarter = st
}

func NewService(
	repo Repository,
	channels channel.Repository,
	contacts contact.Repository,
	senderRepo sender.Repository,
	convRepo conversation.Repository,
	registry *channel.Registry,
	aiRouter *ai.Router,
	leadCtxLoader *leadctx.Loader,
	pool *pgxpool.Pool,
	publicAPIURL string,
) *Service {
	return &Service{
		repo: repo, channels: channels, contacts: contacts,
		sender: senderRepo, conversation: convRepo, registry: registry,
		ai: aiRouter, leadCtx: leadCtxLoader, pool: pool,
		publicAPIURL: publicAPIURL,
	}
}

// CreateCampaign persists a draft campaign. The caller picks a channel; if
// they don't, we fall back to the user's default Gmail channel.
func (s *Service) CreateCampaign(ctx context.Context, userID uuid.UUID, c domain.Campaign) (*domain.Campaign, error) {
	if c.Name == "" {
		return nil, errors.New("campaign name is required")
	}
	// Validate a caller-provided channel (must be the user's + enabled), and
	// fall back to auto-pick for any channel TYPE (gmail_oauth, smtp, …) — not
	// just Gmail. A stale brand default / deleted channel resets to auto-pick.
	// An account that exists but is missing its stored credentials is a HARD
	// error — silently sending from a different account than the one the user
	// picked is never acceptable.
	if c.ChannelID != uuid.Nil {
		ch, err := s.channels.Get(ctx, userID, c.ChannelID)
		switch {
		case err != nil || ch == nil || !ch.Enabled:
			c.ChannelID = uuid.Nil
		case !ch.SendReady():
			return nil, fmt.Errorf("%s isn't fully connected — reconnect it in Settings → Email channels", ch.FromEmail)
		}
	}
	if c.ChannelID == uuid.Nil {
		chs, err := s.channels.List(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, ch := range chs { // prefer the default enabled account
			if ch.Enabled && ch.IsDefault && ch.SendReady() {
				c.ChannelID = ch.ID
				break
			}
		}
		if c.ChannelID == uuid.Nil {
			for _, ch := range chs { // else any enabled account, any type
				if ch.Enabled && ch.SendReady() {
					c.ChannelID = ch.ID
					break
				}
			}
		}
		if c.ChannelID == uuid.Nil {
			return nil, errors.New("no email account connected — connect one in Settings → Email channels first")
		}
	}
	c.UserID = userID
	c.Status = domain.CampaignStatusDraft
	return s.repo.Create(ctx, c)
}

// AddContacts inserts pending campaign_contacts. Contacts that are already
// in another active conversation for this user are skipped unless force=true.
type AddContactsResult struct {
	Added           int         `json:"added"`
	SkippedOverlap  []uuid.UUID `json:"skipped_overlap"`
	SkippedDup      int         `json:"skipped_already_in_campaign"`
}

func (s *Service) AddContacts(ctx context.Context, userID, campaignID uuid.UUID, contactIDs []uuid.UUID, marketID *uuid.UUID, force bool) (*AddContactsResult, error) {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return nil, err
	}
	if camp.Status != domain.CampaignStatusDraft && camp.Status != domain.CampaignStatusReady {
		return nil, fmt.Errorf("can't add contacts to a campaign in status %s", camp.Status)
	}

	var skippedOverlap []uuid.UUID
	target := contactIDs
	if !force {
		overlap, err := s.repo.OverlapWith(ctx, userID, contactIDs)
		if err != nil {
			return nil, err
		}
		if len(overlap) > 0 {
			skipSet := make(map[uuid.UUID]bool, len(overlap))
			for _, id := range overlap {
				skipSet[id] = true
				skippedOverlap = append(skippedOverlap, id)
			}
			target = target[:0]
			for _, id := range contactIDs {
				if !skipSet[id] {
					target = append(target, id)
				}
			}
		}
	}

	added, err := s.repo.AddContacts(ctx, campaignID, target, marketID)
	if err != nil {
		return nil, err
	}
	return &AddContactsResult{
		Added:          added,
		SkippedOverlap: skippedOverlap,
		SkippedDup:     len(target) - added,
	}, nil
}

// ApproveContact flips one row from drafted → approved. If the campaign
// is already running (status=active), it also assigns a scheduled_send_at
// at the next available pace slot — otherwise the row would have NULL
// scheduled_send_at and the scheduler would never pick it up. Before
// launch, scheduled_send_at stays NULL and Launch fills it in.
func (s *Service) ApproveContact(ctx context.Context, userID, campaignID, contactID uuid.UUID) error {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return err
	}
	if camp == nil {
		return domain.ErrNotFound
	}
	cc, err := s.repo.GetContact(ctx, campaignID, contactID)
	if err != nil {
		return err
	}
	// Failed rows are re-approvable: after the user fixes the cause (e.g.
	// reconnects the sending account) this is the "Retry" path.
	if cc.Status != domain.CampaignContactDrafted && cc.Status != domain.CampaignContactFailed {
		return fmt.Errorf("can only approve drafted or failed rows (status=%s)", cc.Status)
	}

	var scheduledAt *time.Time
	if camp.Status == domain.CampaignStatusActive {
		t, err := s.nextPaceSlot(ctx, camp)
		if err != nil {
			return err
		}
		scheduledAt = &t
	}
	if err := s.repo.UpdateContactStatus(ctx, campaignID, contactID, domain.CampaignContactApproved, nil, scheduledAt); err != nil {
		return err
	}
	if cc.Status == domain.CampaignContactFailed {
		// Clear the stale failure reason so the UI doesn't show it on a
		// row that's back in the queue.
		_, _ = s.pool.Exec(ctx,
			`UPDATE campaign_contacts SET skip_reason = NULL WHERE campaign_id = $1 AND contact_id = $2`,
			campaignID, contactID,
		)
	}
	return nil
}

// nextPaceSlot computes the next scheduled_send_at for a campaign that's
// already running. Reads the latest scheduled_send_at across the
// campaign's rows (sent + still-pending sends count, since pace is about
// outbound rate not row order). Returns now if no slots assigned yet
// (shouldn't happen for an active campaign but defensive).
func (s *Service) nextPaceSlot(ctx context.Context, camp *domain.Campaign) (time.Time, error) {
	pace := camp.SendPacePerDay
	if pace <= 0 {
		pace = 50
	}
	gap := time.Duration(86400/pace) * time.Second

	var latest *time.Time
	err := s.pool.QueryRow(ctx,
		`SELECT MAX(scheduled_send_at) FROM campaign_contacts
		 WHERE campaign_id = $1 AND scheduled_send_at IS NOT NULL`,
		camp.ID,
	).Scan(&latest)
	if err != nil {
		return time.Time{}, fmt.Errorf("read latest slot: %w", err)
	}
	now := time.Now().UTC()
	if latest == nil || latest.Before(now) {
		return now, nil
	}
	return latest.Add(gap), nil
}

// ApproveAll flips every drafted row in the campaign to approved.
// If the campaign is already running, each newly-approved row gets the
// next pace slot so they fire in order with the existing schedule.
func (s *Service) ApproveAll(ctx context.Context, userID, campaignID uuid.UUID) (int, error) {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return 0, err
	}

	// When active, schedule each newly-approved row at the next available
	// slot one-by-one so pace is preserved across the batch.
	if camp.Status == domain.CampaignStatusActive {
		drafted, err := s.repo.ListContacts(ctx, camp.ID, domain.CampaignContactDrafted)
		if err != nil {
			return 0, err
		}
		pace := camp.SendPacePerDay
		if pace <= 0 {
			pace = 50
		}
		gap := time.Duration(86400/pace) * time.Second
		base, err := s.nextPaceSlot(ctx, camp)
		if err != nil {
			return 0, err
		}
		for i, row := range drafted {
			t := base.Add(gap * time.Duration(i))
			if err := s.repo.UpdateContactStatus(ctx, camp.ID, row.ContactID, domain.CampaignContactApproved, nil, &t); err != nil {
				return i, err
			}
		}
		return len(drafted), nil
	}

	tag, err := s.pool.Exec(ctx,
		`UPDATE campaign_contacts SET status = 'approved'
		 WHERE campaign_id = $1 AND status = 'drafted'`,
		camp.ID,
	)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// Launch validates compliance prerequisites, schedules each approved row,
// and flips the campaign to active.
func (s *Service) Launch(ctx context.Context, userID, campaignID uuid.UUID) error {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return err
	}
	if camp.Status != domain.CampaignStatusDraft && camp.Status != domain.CampaignStatusReady {
		return fmt.Errorf("can't launch campaign in status %s", camp.Status)
	}

	// Compliance gate: physical address required (on the campaign's brand).
	sp, _ := s.resolveBrand(ctx, camp)
	if sp == nil || sp.PhysicalAddress == "" {
		return errors.New("sender profile is missing a physical address — required by CAN-SPAM/GDPR for bulk sending")
	}

	// Pace: send_pace_per_day approved rows per day, evenly spaced.
	pace := camp.SendPacePerDay
	if pace <= 0 {
		pace = 50
	}
	gap := time.Duration(86400/pace) * time.Second

	// Iterate every approved row and assign scheduled_send_at.
	rows, err := s.repo.ListContacts(ctx, campaignID, domain.CampaignContactApproved)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("no approved contacts to launch")
	}
	now := time.Now().UTC()
	if camp.StartAt != nil && camp.StartAt.After(now) {
		now = *camp.StartAt
	}
	for i, row := range rows {
		t := now.Add(gap * time.Duration(i))
		if err := s.repo.UpdateContactStatus(ctx, campaignID, row.ContactID, domain.CampaignContactApproved, nil, &t); err != nil {
			return err
		}
	}

	startedAt := time.Now().UTC()
	return s.repo.UpdateStatus(ctx, userID, campaignID, domain.CampaignStatusActive, &startedAt, nil)
}

// Pause halts the whole group — cold sends (the scheduler gates on
// status='active') AND follow-ups (the sequence engine's DueRuns excludes
// non-active campaigns). paused_at records when, so Resume can "thaw" correctly.
func (s *Service) Pause(ctx context.Context, userID, campaignID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE campaigns SET status='paused', paused_at=now(), updated_at=now()
		  WHERE id=$1 AND user_id=$2 AND status='active'`, campaignID, userID)
	return err
}

// Resume "thaws" a paused group: it shifts every pending scheduled cold send AND
// every active follow-up run FORWARD by exactly how long the group was paused, so
// nothing fires retroactively (no burst) and the original spacing is preserved —
// the group continues exactly where it left off.
func (s *Service) Resume(ctx context.Context, userID, campaignID uuid.UUID) error {
	var pausedAt *time.Time
	if err := s.pool.QueryRow(ctx,
		`SELECT paused_at FROM campaigns WHERE id=$1 AND user_id=$2`, campaignID, userID,
	).Scan(&pausedAt); err != nil {
		return err
	}

	deltaSecs := 0.0
	if pausedAt != nil {
		if d := time.Since(*pausedAt).Seconds(); d > 0 {
			deltaSecs = d
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Shift unsent cold sends forward by the pause duration.
	if _, err := tx.Exec(ctx,
		`UPDATE campaign_contacts
		    SET scheduled_send_at = scheduled_send_at + make_interval(secs => $2)
		  WHERE campaign_id=$1 AND status='approved' AND scheduled_send_at IS NOT NULL`,
		campaignID, deltaSecs); err != nil {
		return fmt.Errorf("shift cold schedule: %w", err)
	}
	// Shift active follow-up runs for this group's conversations.
	if _, err := tx.Exec(ctx,
		`UPDATE sequence_runs
		    SET next_run_at = next_run_at + make_interval(secs => $2), updated_at=now()
		  WHERE status='active'
		    AND conversation_id IN (SELECT id FROM conversations WHERE campaign_id=$1)`,
		campaignID, deltaSecs); err != nil {
		return fmt.Errorf("shift follow-up runs: %w", err)
	}
	// Reactivate.
	if _, err := tx.Exec(ctx,
		`UPDATE campaigns SET status='active', paused_at=NULL, updated_at=now()
		  WHERE id=$1 AND user_id=$2`, campaignID, userID); err != nil {
		return fmt.Errorf("reactivate campaign: %w", err)
	}
	return tx.Commit(ctx)
}

// RetryFailed flips every status='failed' row back to 'approved' and
// re-assigns a scheduled_send_at at the next pace slot, so failed sends
// (typically caused by a revoked OAuth token that's now been reconnected)
// can fire on the next scheduler tick. Returns the number of rows retried.
func (s *Service) RetryFailed(ctx context.Context, userID, campaignID uuid.UUID) (int, error) {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return 0, err
	}

	failed, err := s.repo.ListContacts(ctx, camp.ID, domain.CampaignContactFailed)
	if err != nil {
		return 0, err
	}
	if len(failed) == 0 {
		return 0, nil
	}

	pace := camp.SendPacePerDay
	if pace <= 0 {
		pace = 50
	}
	gap := time.Duration(86400/pace) * time.Second
	base, err := s.nextPaceSlot(ctx, camp)
	if err != nil {
		return 0, err
	}
	for i, row := range failed {
		t := base.Add(gap * time.Duration(i))
		if err := s.repo.UpdateContactStatus(ctx, camp.ID, row.ContactID, domain.CampaignContactApproved, nil, &t); err != nil {
			return i, err
		}
		// Clear the old failure note so the UI doesn't keep showing
		// "Send failed: <token revoked>" alongside the new pending send.
		if _, err := s.pool.Exec(ctx,
			`UPDATE campaign_contacts SET skip_reason = NULL WHERE campaign_id = $1 AND contact_id = $2`,
			camp.ID, row.ContactID,
		); err != nil {
			return i, err
		}
	}

	// If the campaign was stopped earlier, flip it back to active so the
	// scheduler will pick the rows up.
	if camp.Status == domain.CampaignStatusStopped {
		_ = s.repo.UpdateStatus(ctx, userID, campaignID, domain.CampaignStatusActive, nil, nil)
	}
	return len(failed), nil
}

// UpdateDraft persists user edits to a campaign_contact's pending draft.
// Loads the linked draft_message_id, verifies ownership (the campaign
// belongs to userID) and verifies the message is still editable (status
// in 'draft' or 'pending_approval'), then updates messages.subject +
// messages.body_text in place.
func (s *Service) UpdateDraft(ctx context.Context, userID, campaignID, contactID uuid.UUID, subject, body string) error {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return err
	}
	if camp == nil {
		return domain.ErrNotFound
	}
	cc, err := s.repo.GetContact(ctx, campaignID, contactID)
	if err != nil {
		return err
	}
	if cc.DraftMessageID == nil {
		return errors.New("no draft to edit — campaign hasn't drafted this contact yet")
	}
	// Status gate on the campaign_contacts row: only drafted rows are
	// editable. Approved/sent flow out of user control.
	if cc.Status != domain.CampaignContactDrafted {
		return fmt.Errorf("can't edit a %s draft", cc.Status)
	}
	// Status gate on the message: must still be a draft / pending_approval.
	// (DraftMessageID points to messages.id by FK so the row exists.)
	tag, err := s.pool.Exec(ctx,
		`UPDATE messages
		   SET subject = $1, body_text = $2
		 WHERE id = $3
		   AND status IN ('draft', 'pending_approval')`,
		subject, body, *cc.DraftMessageID,
	)
	if err != nil {
		return fmt.Errorf("update draft: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("draft is no longer editable (already approved or sent)")
	}
	return nil
}

func (s *Service) Stop(ctx context.Context, userID, campaignID uuid.UUID) error {
	completed := time.Now().UTC()
	return s.repo.UpdateStatus(ctx, userID, campaignID, domain.CampaignStatusStopped, nil, &completed)
}

// PositioningOverrideJSON unmarshals the optional positioning override into
// the typed shape the AI prompt expects.
type PositioningOverride struct {
	ProductDescription string `json:"product_description,omitempty"`
	ValueProp          string `json:"value_prop,omitempty"`
	Tone               string `json:"tone,omitempty"`
	Goal               string `json:"goal,omitempty"`
}

func (s *Service) parsePositioning(camp *domain.Campaign) *PositioningOverride {
	if len(camp.PositioningOverride) == 0 {
		return nil
	}
	var p PositioningOverride
	if err := json.Unmarshal(camp.PositioningOverride, &p); err != nil {
		return nil
	}
	return &p
}

// resolveBrand returns the sender profile (brand) a campaign sends as. Uses
// the campaign's assigned SenderProfileID when set, falling back to the
// user's Default brand for legacy campaigns created before per-market brands.
func (s *Service) resolveBrand(ctx context.Context, camp *domain.Campaign) (*domain.SenderProfile, error) {
	if camp != nil && camp.SenderProfileID != nil {
		sp, err := s.sender.GetByID(ctx, camp.UserID, *camp.SenderProfileID)
		if err == nil {
			return sp, nil
		}
		// Fall through to default if the assigned brand was deleted.
	}
	return s.sender.DefaultForUser(ctx, camp.UserID)
}

// CatalogForCampaign returns the catalog bytes for a campaign's brand (or the
// default brand). Used by the scheduler when attaching a catalog at send time.
func (s *Service) CatalogForCampaign(ctx context.Context, camp *domain.Campaign) (filename, mimeType string, data []byte, err error) {
	sp, err := s.resolveBrand(ctx, camp)
	if err != nil || sp == nil {
		return "", "", nil, domain.ErrNotFound
	}
	return s.sender.GetCatalogData(ctx, sp.ID)
}
