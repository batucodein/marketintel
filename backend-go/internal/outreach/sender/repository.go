package sender

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.SenderProfile, error)
	Upsert(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error)
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) Get(ctx context.Context, userID uuid.UUID) (*domain.SenderProfile, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT user_id, company_name, product_description, value_prop,
		        target_buyer_description, tone, signature, default_channel_id, updated_at
		 FROM sender_profiles WHERE user_id = $1`, userID,
	)
	var p domain.SenderProfile
	err := row.Scan(&p.UserID, &p.CompanyName, &p.ProductDescription, &p.ValueProp,
		&p.TargetBuyerDescription, &p.Tone, &p.Signature, &p.DefaultChannelID, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get sender profile: %w", err)
	}
	return &p, nil
}

func (r *repository) Upsert(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error) {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sender_profiles (user_id, company_name, product_description, value_prop,
		                              target_buyer_description, tone, signature, default_channel_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (user_id) DO UPDATE SET
		   company_name = EXCLUDED.company_name,
		   product_description = EXCLUDED.product_description,
		   value_prop = EXCLUDED.value_prop,
		   target_buyer_description = EXCLUDED.target_buyer_description,
		   tone = EXCLUDED.tone,
		   signature = EXCLUDED.signature,
		   default_channel_id = EXCLUDED.default_channel_id,
		   updated_at = now()`,
		p.UserID, p.CompanyName, p.ProductDescription, p.ValueProp,
		p.TargetBuyerDescription, p.Tone, p.Signature, p.DefaultChannelID,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert sender profile: %w", err)
	}
	return r.Get(ctx, p.UserID)
}
