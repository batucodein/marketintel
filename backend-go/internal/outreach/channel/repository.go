package channel

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
)

type Repository interface {
	Create(ctx context.Context, uc domain.UserChannel) (*domain.UserChannel, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*domain.UserChannel, error)
	GetByFromEmail(ctx context.Context, userID uuid.UUID, email string) (*domain.UserChannel, error)
	List(ctx context.Context, userID uuid.UUID) ([]domain.UserChannel, error)
	ListEnabled(ctx context.Context) ([]domain.UserChannel, error)
	UpdateTokens(ctx context.Context, id uuid.UUID, accessCipher, refreshCipher string, expiresAt *time.Time) error
	UpdateConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) error
	UpdateLastPoll(ctx context.Context, id uuid.UUID, at time.Time) error
	Delete(ctx context.Context, userID, id uuid.UUID) error
	SetDefault(ctx context.Context, userID, id uuid.UUID) error
}

type repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) Create(ctx context.Context, uc domain.UserChannel) (*domain.UserChannel, error) {
	uc.ID = uuid.New()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generate unsubscribe secret: %w", err)
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO user_channels (
			id, user_id, type, display_label, from_email,
			oauth_access_token_encrypted, oauth_refresh_token_encrypted,
			oauth_expires_at, oauth_scope, config_encrypted, enabled, is_default, unsubscribe_secret
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		uc.ID, uc.UserID, uc.Type, uc.DisplayLabel, uc.FromEmail,
		uc.OAuthAccessTokenCipher, uc.OAuthRefreshTokenCipher,
		uc.OAuthExpiresAt, uc.OAuthScope, uc.ConfigCipher, uc.Enabled, uc.IsDefault, secret,
	)
	if err != nil {
		return nil, fmt.Errorf("insert user_channel: %w", err)
	}
	return r.Get(ctx, uc.UserID, uc.ID)
}

func (r *repository) Get(ctx context.Context, userID, id uuid.UUID) (*domain.UserChannel, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, type, display_label, from_email,
		        oauth_access_token_encrypted, oauth_refresh_token_encrypted,
		        oauth_expires_at, oauth_scope, config_encrypted, unsubscribe_secret,
		        enabled, is_default, last_poll_at, created_at, updated_at
		 FROM user_channels WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
}

func (r *repository) GetByFromEmail(ctx context.Context, userID uuid.UUID, email string) (*domain.UserChannel, error) {
	return r.scanOne(ctx,
		`SELECT id, user_id, type, display_label, from_email,
		        oauth_access_token_encrypted, oauth_refresh_token_encrypted,
		        oauth_expires_at, oauth_scope, config_encrypted, unsubscribe_secret,
		        enabled, is_default, last_poll_at, created_at, updated_at
		 FROM user_channels WHERE user_id = $1 AND from_email = $2 LIMIT 1`,
		userID, email,
	)
}

func (r *repository) scanOne(ctx context.Context, q string, args ...any) (*domain.UserChannel, error) {
	var c domain.UserChannel
	err := r.pool.QueryRow(ctx, q, args...).Scan(
		&c.ID, &c.UserID, &c.Type, &c.DisplayLabel, &c.FromEmail,
		&c.OAuthAccessTokenCipher, &c.OAuthRefreshTokenCipher,
		&c.OAuthExpiresAt, &c.OAuthScope, &c.ConfigCipher, &c.UnsubscribeSecret,
		&c.Enabled, &c.IsDefault, &c.LastPollAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user_channel: %w", err)
	}
	return &c, nil
}

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]domain.UserChannel, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, type, display_label, from_email,
		        oauth_access_token_encrypted, oauth_refresh_token_encrypted,
		        oauth_expires_at, oauth_scope, config_encrypted, unsubscribe_secret,
		        enabled, is_default, last_poll_at, created_at, updated_at
		 FROM user_channels WHERE user_id = $1 ORDER BY is_default DESC, created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list user_channels: %w", err)
	}
	defer rows.Close()
	var out []domain.UserChannel
	for rows.Next() {
		var c domain.UserChannel
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.Type, &c.DisplayLabel, &c.FromEmail,
			&c.OAuthAccessTokenCipher, &c.OAuthRefreshTokenCipher,
			&c.OAuthExpiresAt, &c.OAuthScope, &c.ConfigCipher, &c.UnsubscribeSecret,
			&c.Enabled, &c.IsDefault, &c.LastPollAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan user_channel row: %w", err)
		}
		out = append(out, c)
	}
	return out, nil
}

// UpdateTokens refreshes the OAuth credentials and re-enables the channel.
// Re-enabling here means a soft-disabled channel (from a previous Delete)
// comes back to life when the user re-OAuths the same email — preserving
// its conversation history.
func (r *repository) UpdateTokens(ctx context.Context, id uuid.UUID, accessCipher, refreshCipher string, expiresAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_channels SET
		   oauth_access_token_encrypted = $1,
		   oauth_refresh_token_encrypted = COALESCE(NULLIF($2, ''), oauth_refresh_token_encrypted),
		   oauth_expires_at = $3,
		   enabled = true,
		   updated_at = now()
		 WHERE id = $4`,
		accessCipher, refreshCipher, expiresAt, id,
	)
	return err
}

// UpdateConfig replaces an IMAP/SMTP channel's encrypted config and re-enables it
// (used when a user re-connects / updates credentials for an existing mailbox).
func (r *repository) UpdateConfig(ctx context.Context, id uuid.UUID, config json.RawMessage) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_channels SET config_encrypted = $1, enabled = true, updated_at = now() WHERE id = $2`,
		config, id,
	)
	return err
}

func (r *repository) UpdateLastPoll(ctx context.Context, id uuid.UUID, at time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_channels SET last_poll_at = $1 WHERE id = $2`,
		at, id,
	)
	return err
}

func (r *repository) ListEnabled(ctx context.Context) ([]domain.UserChannel, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, user_id, type, display_label, from_email,
		        oauth_access_token_encrypted, oauth_refresh_token_encrypted,
		        oauth_expires_at, oauth_scope, config_encrypted, unsubscribe_secret,
		        enabled, is_default, last_poll_at, created_at, updated_at
		 FROM user_channels WHERE enabled = true`,
	)
	if err != nil {
		return nil, fmt.Errorf("list enabled channels: %w", err)
	}
	defer rows.Close()
	var out []domain.UserChannel
	for rows.Next() {
		var c domain.UserChannel
		if err := rows.Scan(
			&c.ID, &c.UserID, &c.Type, &c.DisplayLabel, &c.FromEmail,
			&c.OAuthAccessTokenCipher, &c.OAuthRefreshTokenCipher,
			&c.OAuthExpiresAt, &c.OAuthScope, &c.ConfigCipher, &c.UnsubscribeSecret,
			&c.Enabled, &c.IsDefault, &c.LastPollAt, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// Delete is a soft-delete. We never hard-DELETE a channel because the FK
// from conversations.channel_id ON DELETE CASCADE would wipe every prior
// conversation on it — destroying months of history just because the user
// wanted to disconnect. Instead we mark the row disabled, clear OAuth
// tokens (so the channel can't accidentally send), and let the user
// reconnect later via the OAuth flow which finds the row by from_email
// and refreshes the tokens in place.
func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE user_channels SET
		   enabled = false,
		   is_default = false,
		   oauth_access_token_encrypted = NULL,
		   oauth_refresh_token_encrypted = NULL,
		   oauth_expires_at = NULL,
		   updated_at = now()
		 WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	return err
}

func (r *repository) SetDefault(ctx context.Context, userID, id uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE user_channels SET is_default = false, updated_at = now() WHERE user_id = $1 AND is_default = true`,
		userID,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE user_channels SET is_default = true, updated_at = now() WHERE user_id = $1 AND id = $2`,
		userID, id,
	); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
