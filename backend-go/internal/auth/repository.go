package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

// UserRepository is the auth module's data access interface.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	UpdateAPICallsRemaining(ctx context.Context, id uuid.UUID, remaining int) error
}

type userRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) UserRepository {
	return &userRepo{pool: pool}
}

func (r *userRepo) Create(ctx context.Context, u *domain.User) error {
	u.ID = uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, email, hashed_password, company_name, home_country, subscription_tier, api_calls_remaining)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Email, u.HashedPassword, u.CompanyName, u.HomeCountry, u.SubscriptionTier, u.APICallsRemaining,
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}
	return nil
}

func (r *userRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, hashed_password, company_name, home_country, default_product_categories,
		        subscription_tier, api_calls_remaining, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	)
	return scanUser(row)
}

func (r *userRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, email, hashed_password, company_name, home_country, default_product_categories,
		        subscription_tier, api_calls_remaining, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	)
	return scanUser(row)
}

func (r *userRepo) Update(ctx context.Context, u *domain.User) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET company_name = $1, home_country = $2, updated_at = now() WHERE id = $3`,
		u.CompanyName, u.HomeCountry, u.ID,
	)
	return err
}

func (r *userRepo) UpdateAPICallsRemaining(ctx context.Context, id uuid.UUID, remaining int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET api_calls_remaining = $1, updated_at = now() WHERE id = $2`,
		remaining, id,
	)
	return err
}

func scanUser(row pgx.Row) (*domain.User, error) {
	var u domain.User
	err := row.Scan(
		&u.ID, &u.Email, &u.HashedPassword, &u.CompanyName, &u.HomeCountry, &u.DefaultProductCategories,
		&u.SubscriptionTier, &u.APICallsRemaining, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanning user: %w", err)
	}
	return &u, nil
}
