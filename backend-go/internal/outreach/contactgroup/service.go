package contactgroup

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/contact"
)

type Service struct {
	repo     Repository
	contacts contact.Repository
}

func NewService(repo Repository, contacts contact.Repository) *Service {
	return &Service{repo: repo, contacts: contacts}
}

// AddBusinessesResult reports the dedup buckets so the UI can show what
// happened, plus how many were newly added to the group.
type AddBusinessesResult struct {
	contact.BulkEnsureResult
	AddedToGroup int `json:"added_to_group"`
}

// AddBusinesses promotes a set of market-lead businesses into contacts (dedup:
// one contact per business) and adds them to the group (dedup: idempotent
// membership). Businesses with no email are surfaced, not added.
func (s *Service) AddBusinesses(ctx context.Context, userID, groupID uuid.UUID, businessIDs []uuid.UUID) (*AddBusinessesResult, error) {
	if _, err := s.repo.Get(ctx, userID, groupID); err != nil {
		return nil, err
	}
	// Ensure contacts exist for every emailable business (idempotent).
	bulk, err := s.contacts.EnsureBulkFromBusinesses(ctx, userID, businessIDs)
	if err != nil {
		return nil, err
	}
	// Map the businesses that now have contacts (added + already existed) to
	// their contact ids, then add them to the group.
	withContacts := make([]uuid.UUID, 0, len(bulk.Added)+len(bulk.AlreadyExisted))
	for _, item := range bulk.Added {
		withContacts = append(withContacts, item.BusinessID)
	}
	for _, item := range bulk.AlreadyExisted {
		withContacts = append(withContacts, item.BusinessID)
	}
	contactIDs, err := s.contacts.IDsForBusinesses(ctx, userID, withContacts)
	if err != nil {
		return nil, err
	}
	added, err := s.repo.AddMembers(ctx, groupID, contactIDs)
	if err != nil {
		return nil, err
	}
	return &AddBusinessesResult{BulkEnsureResult: bulk, AddedToGroup: added}, nil
}

// CreateAndAdd makes a new group (optionally with a brand) and adds the given
// businesses to it in one call.
func (s *Service) CreateAndAdd(ctx context.Context, userID uuid.UUID, name string, senderProfileID *uuid.UUID, businessIDs []uuid.UUID) (uuid.UUID, *AddBusinessesResult, error) {
	if name == "" {
		return uuid.Nil, nil, errors.New("group name is required")
	}
	g, err := s.repo.Create(ctx, domain.ContactGroup{
		UserID:          userID,
		Name:            name,
		SenderProfileID: senderProfileID,
	})
	if err != nil {
		return uuid.Nil, nil, err
	}
	res, err := s.AddBusinesses(ctx, userID, g.ID, businessIDs)
	if err != nil {
		return g.ID, nil, err
	}
	return g.ID, res, nil
}
