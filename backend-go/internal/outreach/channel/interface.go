// Package channel defines the transport-agnostic interface for sending
// and receiving messages. Today: Gmail via OAuth. Future: Outlook, SMTP,
// WhatsApp Business, LinkedIn, etc.
//
// All outreach flow code (conversations, campaigns, sequences) depends only
// on this interface, never on a concrete channel. To add a new channel,
// implement Channel and register it in registry.go.
package channel

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

// SendRequest is the neutral outbound-message payload.
type SendRequest struct {
	To                  string
	Subject             string
	BodyText            string
	BodyHTML            string
	// For replies: both identify the same thread to the remote service.
	// Gmail uses ThreadID; other channels may use InReplyToExternalID.
	ThreadID            string
	InReplyToExternalID string
	// Attachments included with the message. Empty slice = no attachments.
	Attachments []Attachment
	// UnsubscribeURL, when set, is rendered into the email's
	// List-Unsubscribe header and (for compliant transports) appended to
	// the body. Manual one-off conversation sends leave this empty.
	UnsubscribeURL string
}

// Attachment is a file to include with an outbound message.
type Attachment struct {
	Filename string
	MimeType string
	Data     []byte
}

// SendResult carries the identifiers the remote service returns so we can
// store them on the Message row and correlate incoming replies later.
type SendResult struct {
	ExternalMessageID string
	ExternalThreadID  string
	SentAt            time.Time
}

// IncomingMessage is what ListNewMessages returns — the neutral shape of
// an inbound message, regardless of transport.
type IncomingMessage struct {
	ExternalID          string
	ExternalThreadID    string
	InReplyToExternalID string
	From                string
	To                  string
	Subject             string
	BodyText            string
	BodyHTML            string
	ReceivedAt          time.Time
}

// Channel is the transport a user connects (one row in user_channels).
// Implementations hold the decrypted credentials in memory.
type Channel interface {
	// Type returns the domain.ChannelType* constant this implementation serves.
	Type() string

	// UserChannelID is the DB id of the backing user_channels row.
	UserChannelID() uuid.UUID

	// CanSend reports whether the channel is in a state where Send will work.
	// Returns false for OAuth channels with expired/missing refresh tokens.
	CanSend(ctx context.Context) bool

	// Send delivers the message. Returns the external IDs so the caller can
	// persist them on the Message row.
	Send(ctx context.Context, req SendRequest) (*SendResult, error)

	// ListNewMessages fetches inbound messages received since `since`.
	// Used by the polling worker. May return an empty slice on no news.
	ListNewMessages(ctx context.Context, since time.Time) ([]IncomingMessage, error)
}

// Factory builds a Channel from a stored user_channels row.
// Each channel type registers one factory in registry.go.
type Factory func(ctx context.Context, uc domain.UserChannel) (Channel, error)
