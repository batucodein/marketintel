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
	if c.ChannelID == uuid.Nil {
		chs, err := s.channels.List(ctx, userID)
		if err != nil {
			return nil, err
		}
		for _, ch := range chs {
			if ch.Enabled && ch.Type == domain.ChannelTypeGmailOAuth && ch.IsDefault {
				c.ChannelID = ch.ID
				break
			}
		}
		if c.ChannelID == uuid.Nil {
			for _, ch := range chs {
				if ch.Enabled && ch.Type == domain.ChannelTypeGmailOAuth {
					c.ChannelID = ch.ID
					break
				}
			}
		}
		if c.ChannelID == uuid.Nil {
			return nil, errors.New("no Gmail channel connected — connect one first")
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

// ApproveContact flips one row from drafted → approved.
// scheduled_send_at is computed at Launch, not here.
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
	if cc.Status != domain.CampaignContactDrafted {
		return fmt.Errorf("can only approve drafted rows (status=%s)", cc.Status)
	}
	return s.repo.UpdateContactStatus(ctx, campaignID, contactID, domain.CampaignContactApproved, nil, nil)
}

// ApproveAll flips every drafted row in the campaign to approved.
func (s *Service) ApproveAll(ctx context.Context, userID, campaignID uuid.UUID) (int, error) {
	camp, err := s.repo.Get(ctx, userID, campaignID)
	if err != nil {
		return 0, err
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

	// Compliance gate: physical address required.
	sp, _ := s.sender.Get(ctx, userID)
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

func (s *Service) Pause(ctx context.Context, userID, campaignID uuid.UUID) error {
	return s.repo.UpdateStatus(ctx, userID, campaignID, domain.CampaignStatusPaused, nil, nil)
}

func (s *Service) Resume(ctx context.Context, userID, campaignID uuid.UUID) error {
	return s.repo.UpdateStatus(ctx, userID, campaignID, domain.CampaignStatusActive, nil, nil)
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
