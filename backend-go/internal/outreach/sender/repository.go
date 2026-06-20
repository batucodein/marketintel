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
	// GetByID loads one brand scoped to its owner.
	GetByID(ctx context.Context, userID, id uuid.UUID) (*domain.SenderProfile, error)
	// List returns all of a user's brands, oldest first.
	List(ctx context.Context, userID uuid.UUID) ([]domain.SenderProfile, error)
	// DefaultForUser returns the user's "Default" brand (or the oldest if no
	// brand is literally named Default). Used as the universal fallback for
	// legacy code paths that only have a userID, never nil on a user who has
	// at least one profile.
	DefaultForUser(ctx context.Context, userID uuid.UUID) (*domain.SenderProfile, error)
	Create(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error)
	Update(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error

	// Catalog storage (bytes live separately from the profile JSON), keyed by
	// the brand's profile id.
	SaveCatalog(ctx context.Context, userID, profileID uuid.UUID, filename, mimeType string, data []byte) error
	DeleteCatalog(ctx context.Context, userID, profileID uuid.UUID) error
	GetCatalogData(ctx context.Context, profileID uuid.UUID) (filename, mimeType string, data []byte, err error)

	// Learned reply lessons (per brand), remembered from draft refinements.
	ListLessons(ctx context.Context, userID, brandID uuid.UUID) ([]Lesson, error)
	DeleteLesson(ctx context.Context, userID, lessonID uuid.UUID) error
}

// Lesson is a remembered per-brand reply instruction.
type Lesson struct {
	ID             uuid.UUID `json:"id"`
	Instruction    string    `json:"instruction"`
	MatchTags      []string  `json:"match_tags"`
	MatchSentiment string    `json:"match_sentiment"`
	CreatedAt      time.Time `json:"created_at"`
}

type repository struct {
	pool *pgxpool.Pool
}

func (r *repository) ListLessons(ctx context.Context, userID, brandID uuid.UUID) ([]Lesson, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, instruction, match_tags, match_sentiment, created_at
		   FROM brand_reply_lessons WHERE user_id=$1 AND sender_profile_id=$2
		 ORDER BY created_at DESC`, userID, brandID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Lesson{}
	for rows.Next() {
		var l Lesson
		if err := rows.Scan(&l.ID, &l.Instruction, &l.MatchTags, &l.MatchSentiment, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, nil
}

func (r *repository) DeleteLesson(ctx context.Context, userID, lessonID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM brand_reply_lessons WHERE id=$1 AND user_id=$2`, lessonID, userID)
	return err
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

const senderProfileCols = `id, name, user_id, company_name, product_description, value_prop,
	        target_buyer_description, tone, signature, physical_address,
	        target_industries, target_countries, avoid_countries,
	        min_deal_size_usd, typical_deal_size_usd,
	        deal_breakers, competitive_moats,
	        default_channel_id,
	        catalog_file_name, catalog_mime_type, catalog_size_bytes, catalog_uploaded_at,
	        updated_at`

func scanProfile(row pgx.Row) (*domain.SenderProfile, error) {
	var p domain.SenderProfile
	err := row.Scan(&p.ID, &p.Name, &p.UserID, &p.CompanyName, &p.ProductDescription, &p.ValueProp,
		&p.TargetBuyerDescription, &p.Tone, &p.Signature, &p.PhysicalAddress,
		&p.TargetIndustries, &p.TargetCountries, &p.AvoidCountries,
		&p.MinDealSizeUSD, &p.TypicalDealSizeUSD,
		&p.DealBreakers, &p.CompetitiveMoats,
		&p.DefaultChannelID,
		&p.CatalogFileName, &p.CatalogMimeType, &p.CatalogSizeBytes, &p.CatalogUploadedAt,
		&p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan sender profile: %w", err)
	}
	return &p, nil
}

func (r *repository) GetByID(ctx context.Context, userID, id uuid.UUID) (*domain.SenderProfile, error) {
	return scanProfile(r.pool.QueryRow(ctx,
		`SELECT `+senderProfileCols+` FROM sender_profiles WHERE id = $1 AND user_id = $2`,
		id, userID,
	))
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]domain.SenderProfile, error) {
	// "Default" first, then alphabetical — stable for the brand list UI.
	rows, err := r.pool.Query(ctx,
		`SELECT `+senderProfileCols+` FROM sender_profiles WHERE user_id = $1
		 ORDER BY (name = 'Default') DESC, name`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list sender profiles: %w", err)
	}
	defer rows.Close()

	var out []domain.SenderProfile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

func (r *repository) DefaultForUser(ctx context.Context, userID uuid.UUID) (*domain.SenderProfile, error) {
	// Prefer a brand literally named 'Default'; otherwise the alphabetically
	// first. Either way deterministic for a given user.
	return scanProfile(r.pool.QueryRow(ctx,
		`SELECT `+senderProfileCols+` FROM sender_profiles WHERE user_id = $1
		 ORDER BY (name = 'Default') DESC, name LIMIT 1`,
		userID,
	))
}

func (r *repository) Create(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error) {
	id := uuid.New()
	_, err := r.pool.Exec(ctx,
		`INSERT INTO sender_profiles (id, name, user_id, company_name, product_description, value_prop,
		                              target_buyer_description, tone, signature, physical_address,
		                              target_industries, target_countries, avoid_countries,
		                              min_deal_size_usd, typical_deal_size_usd,
		                              deal_breakers, competitive_moats,
		                              default_channel_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		id, p.Name, p.UserID, p.CompanyName, p.ProductDescription, p.ValueProp,
		p.TargetBuyerDescription, p.Tone, p.Signature, p.PhysicalAddress,
		p.TargetIndustries, p.TargetCountries, p.AvoidCountries,
		p.MinDealSizeUSD, p.TypicalDealSizeUSD,
		p.DealBreakers, p.CompetitiveMoats,
		p.DefaultChannelID,
	)
	if err != nil {
		return nil, fmt.Errorf("create sender profile: %w", err)
	}
	return r.GetByID(ctx, p.UserID, id)
}

func (r *repository) Update(ctx context.Context, p domain.SenderProfile) (*domain.SenderProfile, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE sender_profiles SET
		   name = $3,
		   company_name = $4,
		   product_description = $5,
		   value_prop = $6,
		   target_buyer_description = $7,
		   tone = $8,
		   signature = $9,
		   physical_address = $10,
		   target_industries = $11,
		   target_countries = $12,
		   avoid_countries = $13,
		   min_deal_size_usd = $14,
		   typical_deal_size_usd = $15,
		   deal_breakers = $16,
		   competitive_moats = $17,
		   default_channel_id = $18,
		   updated_at = now()
		 WHERE id = $1 AND user_id = $2`,
		p.ID, p.UserID, p.Name, p.CompanyName, p.ProductDescription, p.ValueProp,
		p.TargetBuyerDescription, p.Tone, p.Signature, p.PhysicalAddress,
		p.TargetIndustries, p.TargetCountries, p.AvoidCountries,
		p.MinDealSizeUSD, p.TypicalDealSizeUSD,
		p.DealBreakers, p.CompetitiveMoats,
		p.DefaultChannelID,
	)
	if err != nil {
		return nil, fmt.Errorf("update sender profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, domain.ErrNotFound
	}
	return r.GetByID(ctx, p.UserID, p.ID)
}

func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM sender_profiles WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("delete sender profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// SaveCatalog writes (or replaces) a brand's catalog. The profile must
// already exist (verified by the owner-scoped WHERE).
func (r *repository) SaveCatalog(ctx context.Context, userID, profileID uuid.UUID, filename, mimeType string, data []byte) error {
	now := time.Now()
	tag, err := r.pool.Exec(ctx,
		`UPDATE sender_profiles SET
		   catalog_file_name = $3,
		   catalog_mime_type = $4,
		   catalog_data = $5,
		   catalog_size_bytes = $6,
		   catalog_uploaded_at = $7,
		   updated_at = now()
		 WHERE id = $1 AND user_id = $2`,
		profileID, userID, filename, mimeType, data, len(data), now,
	)
	if err != nil {
		return fmt.Errorf("save catalog: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *repository) DeleteCatalog(ctx context.Context, userID, profileID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sender_profiles SET
		   catalog_file_name = NULL,
		   catalog_mime_type = NULL,
		   catalog_data = NULL,
		   catalog_size_bytes = NULL,
		   catalog_uploaded_at = NULL,
		   updated_at = now()
		 WHERE id = $1 AND user_id = $2`,
		profileID, userID,
	)
	return err
}

func (r *repository) GetCatalogData(ctx context.Context, profileID uuid.UUID) (filename, mimeType string, data []byte, err error) {
	var (
		name *string
		mime *string
		buf  []byte
	)
	err = r.pool.QueryRow(ctx,
		`SELECT catalog_file_name, catalog_mime_type, catalog_data
		 FROM sender_profiles WHERE id = $1`, profileID,
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
