package sender

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.SenderProfile, error)
	Upsert(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error)

	// Catalog storage (bytes live separately from the profile JSON).
	SaveCatalog(ctx context.Context, userID uuid.UUID, filename, mimeType string, data []byte) error
	DeleteCatalog(ctx context.Context, userID uuid.UUID) error
	GetCatalogData(ctx context.Context, userID uuid.UUID) (filename, mimeType string, data []byte, err error)
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
		        target_buyer_description, tone, signature, physical_address,
		        default_channel_id,
		        catalog_file_name, catalog_mime_type, catalog_size_bytes, catalog_uploaded_at,
		        updated_at
		 FROM sender_profiles WHERE user_id = $1`, userID,
	)
	var p domain.SenderProfile
	err := row.Scan(&p.UserID, &p.CompanyName, &p.ProductDescription, &p.ValueProp,
		&p.TargetBuyerDescription, &p.Tone, &p.Signature, &p.PhysicalAddress,
		&p.DefaultChannelID,
		&p.CatalogFileName, &p.CatalogMimeType, &p.CatalogSizeBytes, &p.CatalogUploadedAt,
		&p.UpdatedAt)
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
		                              target_buyer_description, tone, signature, physical_address,
		                              default_channel_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (user_id) DO UPDATE SET
		   company_name = EXCLUDED.company_name,
		   product_description = EXCLUDED.product_description,
		   value_prop = EXCLUDED.value_prop,
		   target_buyer_description = EXCLUDED.target_buyer_description,
		   tone = EXCLUDED.tone,
		   signature = EXCLUDED.signature,
		   physical_address = EXCLUDED.physical_address,
		   default_channel_id = EXCLUDED.default_channel_id,
		   updated_at = now()`,
		p.UserID, p.CompanyName, p.ProductDescription, p.ValueProp,
		p.TargetBuyerDescription, p.Tone, p.Signature, p.PhysicalAddress, p.DefaultChannelID,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert sender profile: %w", err)
	}
	return r.Get(ctx, p.UserID)
}

// SaveCatalog writes (or replaces) the user's single catalog. Creates a
// minimal sender_profile row if one doesn't exist yet so we don't violate
// NOT NULL constraints on company_name when upserting catalog-only.
func (r *repository) SaveCatalog(ctx context.Context, userID uuid.UUID, filename, mimeType string, data []byte) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sender_profiles (user_id, company_name, catalog_file_name, catalog_mime_type,
		                              catalog_data, catalog_size_bytes, catalog_uploaded_at)
		 VALUES ($1, '', $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id) DO UPDATE SET
		   catalog_file_name = EXCLUDED.catalog_file_name,
		   catalog_mime_type = EXCLUDED.catalog_mime_type,
		   catalog_data = EXCLUDED.catalog_data,
		   catalog_size_bytes = EXCLUDED.catalog_size_bytes,
		   catalog_uploaded_at = EXCLUDED.catalog_uploaded_at,
		   updated_at = now()`,
		userID, filename, mimeType, data, len(data), now,
	)
	if err != nil {
		return fmt.Errorf("save catalog: %w", err)
	}
	return nil
}

func (r *repository) DeleteCatalog(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sender_profiles SET
		   catalog_file_name = NULL,
		   catalog_mime_type = NULL,
		   catalog_data = NULL,
		   catalog_size_bytes = NULL,
		   catalog_uploaded_at = NULL,
		   updated_at = now()
		 WHERE user_id = $1`,
		userID,
	)
	return err
}

func (r *repository) GetCatalogData(ctx context.Context, userID uuid.UUID) (filename, mimeType string, data []byte, err error) {
	var (
		name *string
		mime *string
		buf  []byte
	)
	err = r.pool.QueryRow(ctx,
		`SELECT catalog_file_name, catalog_mime_type, catalog_data
		 FROM sender_profiles WHERE user_id = $1`, userID,
	).Scan(&name, &mime, &buf)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil, domain.ErrNotFound
	}
	if err != nil {
		return "", "", nil, fmt.Errorf("get catalog: %w", err)
	}
	if name == nil || buf == nil || len(buf) == 0 {
		return "", "", nil, domain.ErrNotFound
	}
	mimeStr := "application/octet-stream"
	if mime != nil && *mime != "" {
		mimeStr = *mime
	}
	return *name, mimeStr, buf, nil
}
