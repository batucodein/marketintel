package sequence

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

// Service is the lightweight orchestration layer for sequence CRUD that the
// HTTP handler talks to. The follow-up engine is a separate Tickable in
// engine.go and uses Repository directly.
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// CreateWithSteps inserts the sequence + steps in one call. Steps may have
// their step_number set or auto-assigned in array order.
func (s *Service) CreateWithSteps(ctx context.Context, userID uuid.UUID, body CreateSequenceInput) (*domain.SequenceWithSteps, error) {
	if body.Name == "" {
		return nil, errors.New("name is required")
	}
	if len(body.Steps) == 0 {
		return nil, errors.New("at least one step is required")
	}
	seq := domain.Sequence{
		UserID:      userID,
		Name:        body.Name,
		Description: body.Description,
		IsTemplate:  body.IsTemplate,
	}
	created, err := s.repo.CreateSequence(ctx, seq)
	if err != nil {
		return nil, err
	}
	steps := normalizeSteps(body.Steps)
	if err := s.repo.ReplaceSteps(ctx, created.ID, steps); err != nil {
		return nil, err
	}
	loaded, _ := s.repo.ListSteps(ctx, created.ID)
	return &domain.SequenceWithSteps{Sequence: *created, Steps: loaded}, nil
}

// ReplaceSteps lets the editor save changes; auto-assigns step_number.
func (s *Service) ReplaceSteps(ctx context.Context, userID, sequenceID uuid.UUID, raw []domain.SequenceStep) ([]domain.SequenceStep, error) {
	if _, err := s.repo.GetSequence(ctx, userID, sequenceID); err != nil {
		return nil, err
	}
	if err := s.repo.ReplaceSteps(ctx, sequenceID, normalizeSteps(raw)); err != nil {
		return nil, err
	}
	return s.repo.ListSteps(ctx, sequenceID)
}

// GetWithSteps assembles the API-shape (sequence + steps + active runs count).
func (s *Service) GetWithSteps(ctx context.Context, userID, id uuid.UUID) (*domain.SequenceWithSteps, error) {
	seq, err := s.repo.GetSequence(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	steps, _ := s.repo.ListSteps(ctx, id)
	count, _ := s.repo.ActiveRunsCount(ctx, id)
	return &domain.SequenceWithSteps{Sequence: *seq, Steps: steps, ActiveRunsCount: count}, nil
}

// StartRunForConversation creates a sequence_run pointing at the conversation,
// scheduling step 1 according to that step's wait_days. Idempotent — calling
// twice on the same (sequence, conversation) leaves the existing run alone.
func (s *Service) StartRunForConversation(ctx context.Context, sequenceID, conversationID uuid.UUID) (*domain.SequenceRun, error) {
	steps, err := s.repo.ListSteps(ctx, sequenceID)
	if err != nil {
		return nil, err
	}
	if len(steps) == 0 {
		return nil, errors.New("sequence has no steps")
	}
	first := steps[0]
	when := time.Now().UTC().Add(time.Duration(first.WaitDays) * 24 * time.Hour)
	return s.repo.StartRun(ctx, sequenceID, conversationID, when)
}

// StartCampaignRun is the campaign.SequenceStarter adapter — it discards the
// returned run pointer so the interface stays minimal.
func (s *Service) StartCampaignRun(ctx context.Context, sequenceID, conversationID uuid.UUID) error {
	_, err := s.StartRunForConversation(ctx, sequenceID, conversationID)
	return err
}

// CreateSequenceInput is the request body shape for POST /sequences.
type CreateSequenceInput struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	IsTemplate  bool                   `json:"is_template"`
	Steps       []domain.SequenceStep `json:"steps"`
}

// normalizeSteps sets step_number = index+1 if zero, validates trigger/action.
func normalizeSteps(in []domain.SequenceStep) []domain.SequenceStep {
	out := make([]domain.SequenceStep, len(in))
	copy(out, in)
	for i := range out {
		if out[i].StepNumber == 0 {
			out[i].StepNumber = i + 1
		}
		if out[i].Trigger == "" {
			out[i].Trigger = domain.SequenceTriggerNoReply
		}
		if out[i].Action == "" {
			out[i].Action = domain.SequenceActionSendMessage
		}
		if out[i].WaitDays < 0 {
			out[i].WaitDays = 0
		}
	}
	return out
}
