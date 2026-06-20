package channel

import (
	"context"
	"encoding/json"
)

// Security modes for the IMAP/SMTP transport (all TLS — plaintext is refused).
const (
	SecurityTLS      = "tls"      // implicit TLS (SMTP 465 / IMAP 993)
	SecuritySTARTTLS = "starttls" // STARTTLS (SMTP 587)
)

// SMTPConfig is the per-mailbox IMAP/SMTP connection config. It lives in the
// channel package (not imapsmtp) so the connect handler can reference it without
// importing imapsmtp — which would be an import cycle, since imapsmtp implements
// this package's Channel interface.
type SMTPConfig struct {
	IMAPHost string `json:"imap_host"`
	IMAPPort int    `json:"imap_port"`
	SMTPHost string `json:"smtp_host"`
	SMTPPort int    `json:"smtp_port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Security string `json:"security"` // tls | starttls
}

// EmailConnector validates and encrypts an IMAP/SMTP mailbox config for the
// connect handler. Implemented by the imapsmtp package and injected in main.go,
// keeping the channel package free of IMAP/SMTP transport dependencies.
type EmailConnector interface {
	// Test verifies IMAP login + SMTP auth without sending. Returns a
	// user-facing error on failure.
	Test(ctx context.Context, cfg SMTPConfig) error
	// EncodeConfig encrypts cfg for storage in user_channels.config_encrypted.
	EncodeConfig(cfg SMTPConfig) (json.RawMessage, error)
}
