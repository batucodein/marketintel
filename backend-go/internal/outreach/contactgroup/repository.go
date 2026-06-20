// Package contactgroup manages named, user-owned collections of contacts.
// Email groups are built from a contact group (inheriting its brand + members).
package contactgroup

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

// GroupWithCount is a contact group plus its current member count, for lists.
type GroupWithCount struct {
	domain.ContactGroup
	MemberCount int `json:"member_count"`
}

type Repository interface {
	List(ctx context.Context, userID uuid.UUID) ([]GroupWithCount, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*domain.ContactGroup, error)
	Create(ctx context.Context, g domain.ContactGroup) (*domain.ContactGroup, error)
	SetBrand(ctx context.Context, userID, id uuid.UUID, profileID *uuid.UUID) error
	Delete(ctx context.Context, userID, id uuid.UUID) error
	// AddMembers inserts contact memberships idempotently (ON CONFLICT DO NOTHING).
	// Returns how many were newly added.
	AddMembers(ctx context.Context, groupID uuid.UUID, contactIDs []uuid.UUID) (int, error)
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]GroupWithCount, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT g.id, g.user_id, g.name, g.sender_profile_id, g.created_at, g.updated_at,
		        (SELECT count(*) FROM contact_group_members m WHERE m.contact_group_id = g.id)
		 FROM contact_groups g
		 WHERE g.user_id = $1
		 ORDER BY g.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list contact groups: %w", err)
	}
	defer rows.Close()
	var out []GroupWithCount
	for rows.Next() {
		var g GroupWithCount
		if err := rows.Scan(&g.ID, &g.UserID, &g.Name, &g.SenderProfileID,
			&g.CreatedAt, &g.UpdatedAt, &g.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, nil
}

func (r *repository) Get(ctx context.Context, userID, id uuid.UUID) (*domain.ContactGroup, error) {
	var g domain.ContactGroup
	err := r.pool.QueryRow(ctx,
		`SELECT id, user_id, name, sender_profile_id, created_at, updated_at
		 FROM contact_groups WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&g.ID, &g.UserID, &g.Name, &g.SenderProfileID, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get contact group: %w", err)
	}
	return &g, nil
}

func (r *repository) Create(ctx context.Context, g domain.ContactGroup) (*domain.ContactGroup, error) {
	id := uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO contact_groups (id, user_id, name, sender_profile_id)
		 VALUES ($1, $2, $3, $4)`,
		id, g.UserID, g.Name, g.SenderProfileID,
	)
	if err != nil {
		return nil, fmt.Errorf("create contact group: %w", err)
	}
	return r.Get(ctx, g.UserID, id)
}

func (r *repository) SetBrand(ctx context.Context, userID, id uuid.UUID, profileID *uuid.UUID) error {
	// When setting (not clearing), require the brand belongs to the user.
	if profileID != nil {
		tag, err := r.pool.Exec(ctx,
			`UPDATE contact_groups SET sender_profile_id = $1, updated_at = now()
			 WHERE id = $2 AND user_id = $3
			   AND EXISTS (SELECT 1 FROM sender_profiles WHERE id = $1 AND user_id = $3)`,
			*profileID, id, userID,
		)
		if err != nil {
			return fmt.Errorf("set contact group brand: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE contact_groups SET sender_profile_id = NULL, updated_at = now()
		 WHERE id = $1 AND user_id = $2`, id, userID,
	)
	return err
}

func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM contact_groups WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete contact group: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *repository) AddMembers(ctx context.Context, groupID uuid.UUID, contactIDs []uuid.UUID) (int, error) {
	added := 0
	for _, cid := range contactIDs {
		tag, err := r.pool.Exec(ctx,
			`INSERT INTO contact_group_members (contact_group_id, contact_id)
			 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			groupID, cid,
		)
		if err != nil {
			return added, fmt.Errorf("add member: %w", err)
		}
		added += int(tag.RowsAffected())
	}
	return added, nil
}
