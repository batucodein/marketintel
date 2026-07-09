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
	UnsubscribedAt    *time.Time `json:"unsubscribed_at,omitempty" db:"unsubscribed_at"`
	UnsubscribeReason *string    `json:"unsubscribe_reason,omitempty" db:"unsubscribe_reason"`
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
	UnsubscribeSecret        []byte          `json:"-" db:"unsubscribe_secret"`
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

// SendReady reports whether the channel has the credentials it needs to
// actually send. A channel can be enabled (listed, selectable) yet missing
// its stored credentials — e.g. an SMTP row whose config was never
// persisted, or an OAuth row whose tokens were cleared on disconnect.
// Binding such a channel to a campaign fails only at send time with no
// visible error, so callers must gate on this before binding.
func (uc UserChannel) SendReady() bool {
	switch uc.Type {
	case ChannelTypeSMTP:
		return len(uc.ConfigCipher) > 0
	case ChannelTypeGmailOAuth, ChannelTypeOutlookOAuth:
		return uc.OAuthRefreshTokenCipher != nil && *uc.OAuthRefreshTokenCipher != ""
	default:
		return true
	}
}

// --- SenderProfile ------------------------------------------------------

// SenderProfile is the "what I sell" context used for AI personalization
// AND for AI lead scoring (so leads are ranked against THIS user's ICP,
// not just the market's product). One per user. Campaigns may override
// specific fields.
type SenderProfile struct {
	// ID is the brand's primary key. A user can have many sender profiles
	// (one brand per market). UserID is now just the owner reference.
	ID                     uuid.UUID  `json:"id" db:"id"`
	Name                   string     `json:"name" db:"name"`
	UserID                 uuid.UUID  `json:"user_id" db:"user_id"`
	CompanyName            string     `json:"company_name" db:"company_name"`
	ProductDescription     string     `json:"product_description" db:"product_description"`
	ValueProp              string     `json:"value_prop" db:"value_prop"`
	TargetBuyerDescription string     `json:"target_buyer_description" db:"target_buyer_description"`
	Tone                   string     `json:"tone" db:"tone"`
	Signature              string     `json:"signature" db:"signature"`
	PhysicalAddress        string     `json:"physical_address" db:"physical_address"`
	DefaultChannelID       *uuid.UUID `json:"default_channel_id" db:"default_channel_id"`

	// Targeting fields (added by migration 17). Free-text by design —
	// the AI scorer reads them as natural language so the user can
	// express intent without locking us into rigid taxonomies.
	TargetIndustries    string `json:"target_industries" db:"target_industries"`
	TargetCountries     string `json:"target_countries" db:"target_countries"`
	AvoidCountries      string `json:"avoid_countries" db:"avoid_countries"`
	MinDealSizeUSD      *int64 `json:"min_deal_size_usd" db:"min_deal_size_usd"`
	TypicalDealSizeUSD  *int64 `json:"typical_deal_size_usd" db:"typical_deal_size_usd"`
	DealBreakers        string `json:"deal_breakers" db:"deal_breakers"`
	CompetitiveMoats    string `json:"competitive_moats" db:"competitive_moats"`

	// Catalog metadata. Bytes live in sender_profiles.catalog_data but are
	// NEVER included in the JSON response — fetched separately during send.
	CatalogFileName   *string    `json:"catalog_file_name,omitempty" db:"catalog_file_name"`
	CatalogMimeType   *string    `json:"catalog_mime_type,omitempty" db:"catalog_mime_type"`
	CatalogSizeBytes  *int       `json:"catalog_size_bytes,omitempty" db:"catalog_size_bytes"`
	CatalogUploadedAt *time.Time `json:"catalog_uploaded_at,omitempty" db:"catalog_uploaded_at"`

	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
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

// --- Campaign -----------------------------------------------------------

// Campaign is a named batch of outreach with shared positioning.
type Campaign struct {
	ID                   uuid.UUID       `json:"id" db:"id"`
	UserID               uuid.UUID       `json:"user_id" db:"user_id"`
	ChannelID            uuid.UUID       `json:"channel_id" db:"channel_id"`
	Name                 string          `json:"name" db:"name"`
	Goal                 string          `json:"goal" db:"goal"`
	Status               string          `json:"status" db:"status"`
	PositioningOverride  json.RawMessage `json:"positioning_override,omitempty" db:"positioning_override"`
	SequenceID           *uuid.UUID      `json:"sequence_id" db:"sequence_id"`
	// MarketID scopes the campaign (Email Group) to one market; SenderProfileID
	// is the brand it sends as (resolved from that market at creation). Both
	// nullable so legacy campaigns survive — workers fall back to the user's
	// Default brand when SenderProfileID is nil.
	MarketID             *uuid.UUID      `json:"market_id" db:"market_id"`
	ContactGroupID       *uuid.UUID      `json:"contact_group_id" db:"contact_group_id"`
	SenderProfileID      *uuid.UUID      `json:"sender_profile_id" db:"sender_profile_id"`
	// Response-aware follow-up branches: what to do when a recipient replies.
	OnPositiveAction     string          `json:"on_positive_action" db:"on_positive_action"`
	OnNegativeAction     string          `json:"on_negative_action" db:"on_negative_action"`
	SendPacePerDay       int             `json:"send_pace_per_day" db:"send_pace_per_day"`
	AttachCatalog        bool            `json:"attach_catalog" db:"attach_catalog"`
	StartAt              *time.Time      `json:"start_at" db:"start_at"`
	StartedAt            *time.Time      `json:"started_at" db:"started_at"`
	CompletedAt          *time.Time      `json:"completed_at" db:"completed_at"`
	CreatedAt            time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at" db:"updated_at"`
}

const (
	CampaignStatusDraft     = "draft"
	CampaignStatusReady     = "ready"
	CampaignStatusActive    = "active"
	CampaignStatusPaused    = "paused"
	CampaignStatusCompleted = "completed"
	CampaignStatusStopped   = "stopped"
)

// CampaignContact is the membership row tying a contact to a campaign with
// its per-row drafting/send state.
type CampaignContact struct {
	CampaignID       uuid.UUID  `json:"campaign_id" db:"campaign_id"`
	ContactID        uuid.UUID  `json:"contact_id" db:"contact_id"`
	MarketID         *uuid.UUID `json:"market_id" db:"market_id"`
	Status           string     `json:"status" db:"status"`
	DraftMessageID   *uuid.UUID `json:"draft_message_id" db:"draft_message_id"`
	ConversationID   *uuid.UUID `json:"conversation_id" db:"conversation_id"`
	ScheduledSendAt  *time.Time `json:"scheduled_send_at" db:"scheduled_send_at"`
	SentAt           *time.Time `json:"sent_at" db:"sent_at"`
	RepliedAt        *time.Time `json:"replied_at" db:"replied_at"`
	SkipReason       *string    `json:"skip_reason" db:"skip_reason"`
	AddedAt          time.Time  `json:"added_at" db:"added_at"`
}

const (
	CampaignContactPending  = "pending"
	CampaignContactDrafted  = "drafted"
	CampaignContactApproved = "approved"
	CampaignContactSent     = "sent"
	CampaignContactReplied  = "replied"
	CampaignContactCold     = "cold"
	CampaignContactSkipped  = "skipped"
	CampaignContactFailed   = "failed"
)

// CampaignSummary is the dashboard-friendly view of a campaign with its
// per-status counts, used by the campaign detail page.
type CampaignSummary struct {
	Campaign
	PendingCount  int `json:"pending_count"`
	DraftedCount  int `json:"drafted_count"`
	ApprovedCount int `json:"approved_count"`
	SentCount     int `json:"sent_count"`
	RepliedCount  int `json:"replied_count"`
	ColdCount     int `json:"cold_count"`
	SkippedCount  int `json:"skipped_count"`
	FailedCount   int `json:"failed_count"`
	TotalCount    int `json:"total_count"`
}

// --- Sequence -----------------------------------------------------------

// Sequence is a follow-up playbook (a list of timed steps).
type Sequence struct {
	ID          uuid.UUID `json:"id" db:"id"`
	UserID      uuid.UUID `json:"user_id" db:"user_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	IsTemplate  bool      `json:"is_template" db:"is_template"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// SequenceStep is one step in a sequence.
type SequenceStep struct {
	ID             uuid.UUID `json:"id" db:"id"`
	SequenceID     uuid.UUID `json:"sequence_id" db:"sequence_id"`
	StepNumber     int       `json:"step_number" db:"step_number"`
	WaitDays       int       `json:"wait_days" db:"wait_days"`
	Trigger        string    `json:"trigger" db:"trigger"`
	Action         string    `json:"action" db:"action"`
	PromptOverride *string   `json:"prompt_override" db:"prompt_override"`
	AutoSend       bool      `json:"auto_send" db:"auto_send"`
}

const (
	SequenceTriggerNoReply       = "no_reply"
	SequenceTriggerAnyReply      = "any_reply"
	SequenceTriggerPositiveReply = "positive_reply"
	SequenceTriggerAlways        = "always"

	SequenceActionSendMessage  = "send_message"
	SequenceActionMarkCold     = "mark_cold"
	SequenceActionNotifyUser   = "notify_user"
	SequenceActionAdvanceStage = "advance_stage"
)

// Response-branch actions: what a campaign does when a recipient replies.
const (
	OnPositiveAutoDraftReply = "auto_draft_reply" // AI drafts a reply for approval (default)
	OnPositiveNotify         = "notify"           // flag it; user takes over
	OnPositiveAutoSend       = "auto_send"        // AI replies and sends (needs prior human send)
	OnNegativeMarkCold       = "mark_cold"        // stop + mark the contact cold (default)
	OnNegativeNotify         = "notify"           // flag it; user takes over
)

// Inbound sentiment buckets (mirrors the classifier output).
const (
	SentimentPositive = "positive"
	SentimentNeutral  = "neutral"
	SentimentNegative = "negative"
)

// SequenceWithSteps is the API-facing sequence, used in handler responses.
type SequenceWithSteps struct {
	Sequence
	Steps           []SequenceStep `json:"steps"`
	ActiveRunsCount int            `json:"active_runs_count"`
}

// SequenceRun tracks one conversation's progress through a sequence.
type SequenceRun struct {
	ID             uuid.UUID `json:"id" db:"id"`
	SequenceID     uuid.UUID `json:"sequence_id" db:"sequence_id"`
	ConversationID uuid.UUID `json:"conversation_id" db:"conversation_id"`
	CurrentStep    int       `json:"current_step" db:"current_step"`
	NextRunAt      time.Time `json:"next_run_at" db:"next_run_at"`
	Status         string    `json:"status" db:"status"`
	LastError      *string   `json:"last_error" db:"last_error"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

const (
	SequenceRunActive          = "active"
	SequenceRunPaused          = "paused"
	SequenceRunCompleted       = "completed"
	SequenceRunStoppedOnReply  = "stopped_on_reply"
)

// --- Contact groups -----------------------------------------------------

// ContactGroup is a named, user-owned collection of contacts with its own
// brand. Email groups are built from a contact group.
type ContactGroup struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	UserID          uuid.UUID  `json:"user_id" db:"user_id"`
	Name            string     `json:"name" db:"name"`
	SenderProfileID *uuid.UUID `json:"sender_profile_id" db:"sender_profile_id"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// --- Tasks & notes ------------------------------------------------------

// Task is a CRM to-do, optionally tied to a contact and/or conversation.
type Task struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	UserID         uuid.UUID  `json:"user_id" db:"user_id"`
	ContactID      *uuid.UUID `json:"contact_id" db:"contact_id"`
	ConversationID *uuid.UUID `json:"conversation_id" db:"conversation_id"`
	Title          string     `json:"title" db:"title"`
	Body           string     `json:"body" db:"body"`
	DueAt          *time.Time `json:"due_at" db:"due_at"`
	CompletedAt    *time.Time `json:"completed_at" db:"completed_at"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// Note is a free-text annotation attached to a contact.
type Note struct {
	ID        uuid.UUID `json:"id" db:"id"`
	UserID    uuid.UUID `json:"user_id" db:"user_id"`
	ContactID uuid.UUID `json:"contact_id" db:"contact_id"`
	Body      string    `json:"body" db:"body"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
