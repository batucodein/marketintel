package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// --- Contact ------------------------------------------------------------

// Contact is a user's CRM relationship with a business.
// One contact per (user_id, business_id). Survives across markets.
type Contact struct {
	ID                uuid.UUID  `json:"id" db:"id"`
	UserID            uuid.UUID  `json:"user_id" db:"user_id"`
	BusinessID        uuid.UUID  `json:"business_id" db:"business_id"`
	PrimaryEmail      *string    `json:"primary_email" db:"primary_email"`
	PrimaryPhone      *string    `json:"primary_phone" db:"primary_phone"`
	DisplayName       string     `json:"display_name" db:"display_name"`
	PipelineStage     string     `json:"pipeline_stage" db:"pipeline_stage"`
	DefaultAutomation string     `json:"default_automation" db:"default_automation"`
	DefaultSequenceID *uuid.UUID `json:"default_sequence_id" db:"default_sequence_id"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at" db:"updated_at"`
}

// Pipeline stages — denormalized enum as strings for flexibility.
const (
	PipelineLead       = "lead"
	PipelineContacted  = "contacted"
	PipelineReplied    = "replied"
	PipelineQualified  = "qualified"
	PipelineWon        = "won"
	PipelineLost       = "lost"
)

const (
	AutomationManual = "manual"
	AutomationSemi   = "semi"
	AutomationAuto   = "auto"
)

// --- UserChannel --------------------------------------------------------

// UserChannel is one sending/receiving channel configured by a user.
// Tokens are stored encrypted at rest; the struct holds plaintext in memory.
type UserChannel struct {
	ID                       uuid.UUID       `json:"id" db:"id"`
	UserID                   uuid.UUID       `json:"user_id" db:"user_id"`
	Type                     string          `json:"type" db:"type"`
	DisplayLabel             string          `json:"display_label" db:"display_label"`
	FromEmail                string          `json:"from_email" db:"from_email"`
	OAuthAccessTokenCipher   *string         `json:"-" db:"oauth_access_token_encrypted"`
	OAuthRefreshTokenCipher  *string         `json:"-" db:"oauth_refresh_token_encrypted"`
	OAuthExpiresAt           *time.Time      `json:"oauth_expires_at" db:"oauth_expires_at"`
	OAuthScope               *string         `json:"oauth_scope" db:"oauth_scope"`
	ConfigCipher             json.RawMessage `json:"-" db:"config_encrypted"`
	Enabled                  bool            `json:"enabled" db:"enabled"`
	IsDefault                bool            `json:"is_default" db:"is_default"`
	LastPollAt               *time.Time      `json:"last_poll_at" db:"last_poll_at"`
	CreatedAt                time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at" db:"updated_at"`
}

// Channel type constants.
const (
	ChannelTypeGmailOAuth   = "gmail_oauth"
	ChannelTypeOutlookOAuth = "outlook_oauth"
	ChannelTypeSMTP         = "smtp"
	ChannelTypeWhatsApp     = "whatsapp"
)

// --- SenderProfile ------------------------------------------------------

// SenderProfile is the "what I sell" context used for AI personalization.
// One per user. Campaigns may override specific fields.
type SenderProfile struct {
	UserID                  uuid.UUID  `json:"user_id" db:"user_id"`
	CompanyName             string     `json:"company_name" db:"company_name"`
	ProductDescription      string     `json:"product_description" db:"product_description"`
	ValueProp               string     `json:"value_prop" db:"value_prop"`
	TargetBuyerDescription  string     `json:"target_buyer_description" db:"target_buyer_description"`
	Tone                    string     `json:"tone" db:"tone"`
	Signature               string     `json:"signature" db:"signature"`
	DefaultChannelID        *uuid.UUID `json:"default_channel_id" db:"default_channel_id"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

// --- Conversation -------------------------------------------------------

// Conversation is a threaded message log on a single channel.
type Conversation struct {
	ID               uuid.UUID  `json:"id" db:"id"`
	UserID           uuid.UUID  `json:"user_id" db:"user_id"`
	ContactID        uuid.UUID  `json:"contact_id" db:"contact_id"`
	ChannelID        uuid.UUID  `json:"channel_id" db:"channel_id"`
	CampaignID       *uuid.UUID `json:"campaign_id" db:"campaign_id"`
	ChannelType      string     `json:"channel_type" db:"channel_type"`
	Subject          *string    `json:"subject" db:"subject"`
	ExternalThreadID *string    `json:"external_thread_id" db:"external_thread_id"`
	Automation       string     `json:"automation" db:"automation"`
	Status           string     `json:"status" db:"status"`
	LastMessageAt    *time.Time `json:"last_message_at" db:"last_message_at"`
	LastDirection    *string    `json:"last_direction" db:"last_direction"`
	Unread           bool       `json:"unread" db:"unread"`
	CreatedAt        time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at" db:"updated_at"`
}

const (
	ConversationStatusActive = "active"
	ConversationStatusPaused = "paused"
	ConversationStatusClosed = "closed"

	DirectionOutbound = "out"
	DirectionInbound  = "in"
)

// --- Message ------------------------------------------------------------

// Message is a single message in a conversation, channel-agnostic.
type Message struct {
	ID                    uuid.UUID  `json:"id" db:"id"`
	ConversationID        uuid.UUID  `json:"conversation_id" db:"conversation_id"`
	Direction             string     `json:"direction" db:"direction"`
	ChannelType           string     `json:"channel_type" db:"channel_type"`
	ExternalID            *string    `json:"external_id" db:"external_id"`
	InReplyToExternalID   *string    `json:"in_reply_to_external_id" db:"in_reply_to_external_id"`
	Subject               *string    `json:"subject" db:"subject"`
	BodyText              *string    `json:"body_text" db:"body_text"`
	BodyHTML              *string    `json:"body_html" db:"body_html"`
	AIGenerated           bool       `json:"ai_generated" db:"ai_generated"`
	AIModel               *string    `json:"ai_model" db:"ai_model"`
	AIPromptVersion       *string    `json:"ai_prompt_version" db:"ai_prompt_version"`
	Status                string     `json:"status" db:"status"`
	CampaignID            *uuid.UUID `json:"campaign_id" db:"campaign_id"`
	SequenceStepID        *uuid.UUID `json:"sequence_step_id" db:"sequence_step_id"`
	SentAt                *time.Time `json:"sent_at" db:"sent_at"`
	ReceivedAt            *time.Time `json:"received_at" db:"received_at"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
}

const (
	MessageStatusDraft            = "draft"
	MessageStatusPendingApproval  = "pending_approval"
	MessageStatusQueued           = "queued"
	MessageStatusSent             = "sent"
	MessageStatusBounced          = "bounced"
	MessageStatusOpened           = "opened"
	MessageStatusFailed           = "failed"
)

// --- Combined views for handler responses ------------------------------

// ConversationWithContext is returned in inbox and detail endpoints —
// joins contact info, business name, last message preview.
type ConversationWithContext struct {
	Conversation
	ContactName       string    `json:"contact_name"`
	ContactEmail      *string   `json:"contact_email"`
	BusinessID        uuid.UUID `json:"business_id"`
	BusinessName      string    `json:"business_name"`
	LastMessageSnippet *string  `json:"last_message_snippet"`
	MessageCount       int      `json:"message_count"`
}
