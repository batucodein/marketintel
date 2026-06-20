// Package imapsmtp is the Channel implementation for a generic mailbox over
// IMAP (receive) + SMTP (send) — used to connect any custom-domain / any-provider
// account that the OAuth channels (Gmail, later Outlook) don't cover. Credentials
// are stored encrypted (crypto.Cipher) in user_channels.config_encrypted and
// never leave our infrastructure. All transport is TLS-only.
package imapsmtp

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	imapclient "github.com/emersion/go-imap/client"
	gomessagemail "github.com/emersion/go-message/mail"
	"github.com/google/uuid"
	gomail "github.com/wneessen/go-mail"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/platform/crypto"
)

// Channel implements channel.Channel for a generic IMAP/SMTP mailbox. The
// config type (channel.SMTPConfig) lives in the channel package so the connect
// handler can use it without importing this package (which would be a cycle).
type Channel struct {
	uc  domain.UserChannel
	cfg channel.SMTPConfig
}

// Connector implements channel.EmailConnector — validates + encrypts a mailbox
// config for the connect handler. Wired in main.go.
type Connector struct{ cipher *crypto.Cipher }

func NewConnector(c *crypto.Cipher) *Connector { return &Connector{cipher: c} }

func (k *Connector) Test(ctx context.Context, cfg channel.SMTPConfig) error {
	return TestConnection(ctx, cfg)
}

func (k *Connector) EncodeConfig(cfg channel.SMTPConfig) (json.RawMessage, error) {
	return EncodeConfig(k.cipher, cfg)
}

// NewFactory returns a channel.Factory that decrypts the stored config and
// builds a Channel. Register with channel.DefaultRegistry under ChannelTypeSMTP.
func NewFactory(cipher *crypto.Cipher) channel.Factory {
	return func(ctx context.Context, uc domain.UserChannel) (channel.Channel, error) {
		cfg, err := DecodeConfig(cipher, uc.ConfigCipher)
		if err != nil {
			return nil, fmt.Errorf("imapsmtp: load config: %w", err)
		}
		return &Channel{uc: uc, cfg: cfg}, nil
	}
}

// EncodeConfig encrypts cfg for storage in config_encrypted (a JSON string of
// the ciphertext). Used by the connect handler.
func EncodeConfig(cipher *crypto.Cipher, cfg channel.SMTPConfig) (json.RawMessage, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	enc, err := cipher.Encrypt(string(raw))
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(enc)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// DecodeConfig reverses EncodeConfig.
func DecodeConfig(cipher *crypto.Cipher, stored json.RawMessage) (channel.SMTPConfig, error) {
	if len(stored) == 0 {
		return channel.SMTPConfig{}, errors.New("no config")
	}
	var enc string
	if err := json.Unmarshal(stored, &enc); err != nil {
		return channel.SMTPConfig{}, err
	}
	raw, err := cipher.Decrypt(enc)
	if err != nil {
		return channel.SMTPConfig{}, err
	}
	var cfg channel.SMTPConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return channel.SMTPConfig{}, err
	}
	return cfg, nil
}

func (c *Channel) Type() string            { return domain.ChannelTypeSMTP }
func (c *Channel) UserChannelID() uuid.UUID { return c.uc.ID }

// CanSend reports send readiness. Stored credentials don't expire (the
// durability win over OAuth), so we treat a configured mailbox as sendable;
// a since-revoked password surfaces as a Send error handled downstream.
func (c *Channel) CanSend(ctx context.Context) bool {
	return c.cfg.SMTPHost != "" && c.cfg.Username != "" && c.cfg.Password != ""
}

// Send delivers via SMTP. go-mail builds the MIME + handles TLS. SMTP returns
// no server id, so we generate our own Message-ID and return it so replies can
// be correlated (In-Reply-To → this id).
func (c *Channel) Send(ctx context.Context, req channel.SendRequest) (*channel.SendResult, error) {
	m := gomail.NewMsg()
	if err := m.From(c.uc.FromEmail); err != nil {
		return nil, fmt.Errorf("imapsmtp: from: %w", err)
	}
	if err := m.To(req.To); err != nil {
		return nil, fmt.Errorf("imapsmtp: to: %w", err)
	}
	m.Subject(req.Subject)

	// go-mail's SetMessageIDWithValue wraps the value in angle brackets itself,
	// so pass the bare id and reconstruct the bracketed form we store + return —
	// it must be byte-identical to what a reply's In-Reply-To header will echo,
	// or the poller's In-Reply-To match (ladder step 2) silently misses.
	idValue := fmt.Sprintf("%s@%s", uuid.NewString(), hostFromEmail(c.uc.FromEmail))
	m.SetMessageIDWithValue(idValue)
	msgID := "<" + idValue + ">"
	if req.InReplyToExternalID != "" {
		m.SetGenHeader("In-Reply-To", req.InReplyToExternalID)
		m.SetGenHeader("References", req.InReplyToExternalID)
	}
	if req.UnsubscribeURL != "" {
		m.SetGenHeader("List-Unsubscribe", "<"+req.UnsubscribeURL+">")
		m.SetGenHeader("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}

	switch {
	case req.BodyHTML != "" && req.BodyText != "":
		m.SetBodyString(gomail.TypeTextPlain, req.BodyText)
		m.AddAlternativeString(gomail.TypeTextHTML, req.BodyHTML)
	case req.BodyHTML != "":
		m.SetBodyString(gomail.TypeTextHTML, req.BodyHTML)
	default:
		m.SetBodyString(gomail.TypeTextPlain, req.BodyText)
	}
	for _, att := range req.Attachments {
		if len(att.Data) == 0 || att.Filename == "" {
			continue
		}
		att := att
		if err := m.AttachReader(att.Filename, strings.NewReader(string(att.Data))); err != nil {
			return nil, fmt.Errorf("imapsmtp: attach %s: %w", att.Filename, err)
		}
	}

	cl, err := c.smtpClient()
	if err != nil {
		return nil, err
	}
	if err := cl.DialAndSendWithContext(ctx, m); err != nil {
		return nil, fmt.Errorf("imapsmtp: send: %w", err)
	}
	return &channel.SendResult{ExternalMessageID: msgID, SentAt: time.Now().UTC()}, nil
}

func (c *Channel) smtpClient() (*gomail.Client, error) {
	return newSMTPClient(c.cfg)
}

func newSMTPClient(cfg channel.SMTPConfig) (*gomail.Client, error) {
	opts := []gomail.Option{
		gomail.WithPort(cfg.SMTPPort),
		gomail.WithUsername(cfg.Username),
		gomail.WithPassword(cfg.Password),
		gomail.WithSMTPAuth(gomail.SMTPAuthPlain),
		gomail.WithTimeout(30 * time.Second),
		gomail.WithTLSConfig(&tls.Config{ServerName: cfg.SMTPHost, MinVersion: tls.VersionTLS12}),
	}
	if cfg.Security == channel.SecuritySTARTTLS && cfg.SMTPPort != 465 {
		opts = append(opts, gomail.WithTLSPolicy(gomail.TLSMandatory)) // STARTTLS required
	} else {
		opts = append(opts, gomail.WithSSL()) // implicit TLS (465)
	}
	return gomail.NewClient(cfg.SMTPHost, opts...)
}

// ListNewMessages pulls inbound messages received since `since` over IMAP (993,
// implicit TLS). The poller dedups by ExternalID (Message-ID) and routes via
// the In-Reply-To / From matching ladder (IMAP has no provider thread id).
func (c *Channel) ListNewMessages(ctx context.Context, since time.Time) ([]channel.IncomingMessage, error) {
	addr := fmt.Sprintf("%s:%d", c.cfg.IMAPHost, c.cfg.IMAPPort)
	cl, err := imapclient.DialTLS(addr, &tls.Config{ServerName: c.cfg.IMAPHost, MinVersion: tls.VersionTLS12})
	if err != nil {
		return nil, fmt.Errorf("imapsmtp: imap dial: %w", err)
	}
	defer cl.Logout()
	if err := cl.Login(c.cfg.Username, c.cfg.Password); err != nil {
		return nil, fmt.Errorf("imapsmtp: imap login: %w", err)
	}
	if _, err := cl.Select("INBOX", true); err != nil {
		return nil, fmt.Errorf("imapsmtp: select inbox: %w", err)
	}

	criteria := imap.NewSearchCriteria()
	criteria.Since = since
	ids, err := cl.Search(criteria)
	if err != nil {
		return nil, fmt.Errorf("imapsmtp: search: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	seqset := new(imap.SeqSet)
	seqset.AddNum(ids...)
	section := &imap.BodySectionName{}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchInternalDate, section.FetchItem()}

	messages := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	go func() { done <- cl.Fetch(seqset, items, messages) }()

	var out []channel.IncomingMessage
	for msg := range messages {
		if inc := parseIMAP(msg, section); inc != nil {
			out = append(out, *inc)
		}
	}
	if err := <-done; err != nil {
		return nil, fmt.Errorf("imapsmtp: fetch: %w", err)
	}
	return out, nil
}

// parseIMAP turns a fetched IMAP message into the neutral IncomingMessage shape.
func parseIMAP(msg *imap.Message, section *imap.BodySectionName) *channel.IncomingMessage {
	if msg == nil || msg.Envelope == nil {
		return nil
	}
	env := msg.Envelope
	inc := channel.IncomingMessage{
		ExternalID:          env.MessageId,
		InReplyToExternalID: env.InReplyTo,
		Subject:             env.Subject,
		ReceivedAt:          msg.InternalDate,
	}
	if len(env.From) > 0 {
		inc.From = addrString(env.From[0])
	}
	if len(env.To) > 0 {
		inc.To = addrString(env.To[0])
	}
	if lit := msg.GetBody(section); lit != nil {
		inc.BodyText, inc.BodyHTML = readBody(lit)
	}
	return &inc
}

// readBody walks the MIME message and returns plain + html parts.
func readBody(r io.Reader) (text, html string) {
	mr, err := gomessagemail.CreateReader(r)
	if err != nil {
		// Not MIME-multipart — treat whole body as plain text.
		b, _ := io.ReadAll(r)
		return string(b), ""
	}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch h := part.Header.(type) {
		case *gomessagemail.InlineHeader:
			ct, _, _ := h.ContentType()
			b, _ := io.ReadAll(part.Body)
			if strings.HasPrefix(ct, "text/html") {
				html = string(b)
			} else if strings.HasPrefix(ct, "text/plain") {
				text = string(b)
			}
		}
	}
	return text, html
}

func addrString(a *imap.Address) string {
	if a == nil {
		return ""
	}
	if a.MailboxName == "" || a.HostName == "" {
		return a.PersonalName
	}
	return a.MailboxName + "@" + a.HostName
}

func hostFromEmail(email string) string {
	if i := strings.LastIndex(email, "@"); i >= 0 && i < len(email)-1 {
		return email[i+1:]
	}
	return "localhost"
}

// TestConnection verifies both IMAP login and SMTP auth work, without sending.
// Used by the connect handler before persisting credentials.
func TestConnection(ctx context.Context, cfg channel.SMTPConfig) error {
	// IMAP
	imapCl, err := imapclient.DialTLS(fmt.Sprintf("%s:%d", cfg.IMAPHost, cfg.IMAPPort),
		&tls.Config{ServerName: cfg.IMAPHost, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("IMAP connection failed: %w", err)
	}
	if err := imapCl.Login(cfg.Username, cfg.Password); err != nil {
		_ = imapCl.Logout()
		return fmt.Errorf("IMAP login failed (check username/password — Gmail/Outlook need an app password): %w", err)
	}
	_ = imapCl.Logout()

	// SMTP
	smtpCl, err := newSMTPClient(cfg)
	if err != nil {
		return fmt.Errorf("SMTP setup failed: %w", err)
	}
	if err := smtpCl.DialWithContext(ctx); err != nil {
		return fmt.Errorf("SMTP connection/auth failed: %w", err)
	}
	_ = smtpCl.Close()
	return nil
}
