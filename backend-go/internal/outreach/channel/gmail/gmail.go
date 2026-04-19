// Package gmail is the Channel implementation for Gmail via OAuth.
// Sends through users.messages.send (so the message appears in the user's
// Sent folder) and fetches new inbound messages via users.history.list
// with a fallback to users.messages.list.
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/platform/crypto"
	"github.com/batuhan/marketintel/internal/platform/oauth"
)

// Channel implements channel.Channel for Gmail.
type Channel struct {
	uc          domain.UserChannel
	svc         *gmail.Service
	cipher      *crypto.Cipher
	oauth       *oauth.GmailOAuth
	tokenSource oauth2.TokenSource
}

// NewFactory returns a channel.Factory that builds Gmail Channel instances.
// The factory needs the OAuth helper (for token refresh) and the cipher
// (to decrypt stored tokens). Register this with channel.DefaultRegistry.
func NewFactory(o *oauth.GmailOAuth, c *crypto.Cipher) channel.Factory {
	return func(ctx context.Context, uc domain.UserChannel) (channel.Channel, error) {
		if uc.OAuthAccessTokenCipher == nil || *uc.OAuthAccessTokenCipher == "" {
			return nil, errors.New("gmail: channel has no access token")
		}
		access, err := c.Decrypt(*uc.OAuthAccessTokenCipher)
		if err != nil {
			return nil, fmt.Errorf("gmail: decrypt access token: %w", err)
		}
		refresh := ""
		if uc.OAuthRefreshTokenCipher != nil {
			refresh, err = c.Decrypt(*uc.OAuthRefreshTokenCipher)
			if err != nil {
				return nil, fmt.Errorf("gmail: decrypt refresh token: %w", err)
			}
		}

		tok := &oauth2.Token{
			AccessToken:  access,
			RefreshToken: refresh,
			TokenType:    "Bearer",
		}
		if uc.OAuthExpiresAt != nil {
			tok.Expiry = *uc.OAuthExpiresAt
		}

		ts := o.TokenSource(ctx, tok)
		svc, err := gmail.NewService(ctx, option.WithTokenSource(ts))
		if err != nil {
			return nil, fmt.Errorf("gmail: create service: %w", err)
		}

		return &Channel{
			uc:          uc,
			svc:         svc,
			cipher:      c,
			oauth:       o,
			tokenSource: ts,
		}, nil
	}
}

func (c *Channel) Type() string              { return domain.ChannelTypeGmailOAuth }
func (c *Channel) UserChannelID() uuid.UUID   { return c.uc.ID }
func (c *Channel) CanSend(ctx context.Context) bool {
	_, err := c.tokenSource.Token()
	return err == nil
}

// Send composes and sends via users.messages.send. Returns Gmail's
// generated message_id and thread_id so the caller can persist them.
func (c *Channel) Send(ctx context.Context, req channel.SendRequest) (*channel.SendResult, error) {
	raw, err := buildRFC5322(req, c.uc.FromEmail)
	if err != nil {
		return nil, fmt.Errorf("gmail: build message: %w", err)
	}

	msg := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString(raw),
	}
	if req.ThreadID != "" {
		msg.ThreadId = req.ThreadID
	}

	sent, err := c.svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: send: %w", err)
	}

	return &channel.SendResult{
		ExternalMessageID: sent.Id,
		ExternalThreadID:  sent.ThreadId,
		SentAt:            time.Now().UTC(),
	}, nil
}

// ListNewMessages pulls messages received after `since`.
// Uses search with newer_than clause — simpler than history API for P1.
func (c *Channel) ListNewMessages(ctx context.Context, since time.Time) ([]channel.IncomingMessage, error) {
	// Build query: unread + newer than since. We scope to INBOX to avoid pulling drafts, etc.
	query := fmt.Sprintf("in:inbox after:%d", since.Unix())
	list, err := c.svc.Users.Messages.List("me").Q(query).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail: list messages: %w", err)
	}

	out := make([]channel.IncomingMessage, 0, len(list.Messages))
	for _, ref := range list.Messages {
		m, err := c.svc.Users.Messages.Get("me", ref.Id).
			Format("metadata").
			MetadataHeaders("From", "To", "Subject", "Date", "Message-ID", "In-Reply-To").
			Context(ctx).Do()
		if err != nil {
			continue
		}
		full, err := c.svc.Users.Messages.Get("me", ref.Id).Format("full").Context(ctx).Do()
		if err != nil {
			continue
		}
		incoming := parseMessage(m, full)
		if incoming == nil {
			continue
		}
		out = append(out, *incoming)
	}
	return out, nil
}

// --- helpers ------------------------------------------------------------

// buildRFC5322 constructs a minimal RFC 5322 message body suitable for
// Gmail's users.messages.send. We build text + html parts if both supplied.
func buildRFC5322(req channel.SendRequest, fromEmail string) ([]byte, error) {
	if req.To == "" {
		return nil, errors.New("missing To")
	}
	if req.Subject == "" {
		return nil, errors.New("missing Subject")
	}
	if req.BodyText == "" && req.BodyHTML == "" {
		return nil, errors.New("missing body")
	}

	var sb strings.Builder
	sb.WriteString("From: " + fromEmail + "\r\n")
	sb.WriteString("To: " + req.To + "\r\n")
	sb.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", req.Subject) + "\r\n")
	if req.InReplyToExternalID != "" {
		sb.WriteString("In-Reply-To: " + req.InReplyToExternalID + "\r\n")
		sb.WriteString("References: " + req.InReplyToExternalID + "\r\n")
	}
	sb.WriteString("MIME-Version: 1.0\r\n")

	if req.BodyHTML != "" && req.BodyText != "" {
		boundary := "mi_" + randomBoundary()
		sb.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		sb.WriteString(req.BodyText + "\r\n")
		sb.WriteString("--" + boundary + "\r\n")
		sb.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		sb.WriteString(req.BodyHTML + "\r\n")
		sb.WriteString("--" + boundary + "--\r\n")
	} else if req.BodyHTML != "" {
		sb.WriteString("Content-Type: text/html; charset=utf-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		sb.WriteString(req.BodyHTML)
	} else {
		sb.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		sb.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
		sb.WriteString(req.BodyText)
	}

	return []byte(sb.String()), nil
}

func randomBoundary() string {
	b, _ := json.Marshal(time.Now().UnixNano())
	return base64.RawURLEncoding.EncodeToString(b)
}

// parseMessage converts a Gmail message (metadata + full) into our neutral shape.
func parseMessage(meta, full *gmail.Message) *channel.IncomingMessage {
	headers := make(map[string]string)
	if meta.Payload != nil {
		for _, h := range meta.Payload.Headers {
			headers[strings.ToLower(h.Name)] = h.Value
		}
	}
	bodyText, bodyHTML := extractBody(full.Payload)

	received := time.UnixMilli(meta.InternalDate)

	return &channel.IncomingMessage{
		ExternalID:          headers["message-id"],
		ExternalThreadID:    meta.ThreadId,
		InReplyToExternalID: headers["in-reply-to"],
		From:                headers["from"],
		To:                  headers["to"],
		Subject:             headers["subject"],
		BodyText:            bodyText,
		BodyHTML:            bodyHTML,
		ReceivedAt:          received,
	}
}

// extractBody recursively walks the MIME tree and returns plain+html parts.
func extractBody(p *gmail.MessagePart) (text, html string) {
	if p == nil {
		return "", ""
	}
	if p.Body != nil && p.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(p.Body.Data)
		if err == nil {
			switch {
			case strings.HasPrefix(p.MimeType, "text/plain"):
				return string(decoded), ""
			case strings.HasPrefix(p.MimeType, "text/html"):
				return "", string(decoded)
			}
		}
	}
	for _, part := range p.Parts {
		t, h := extractBody(part)
		if t != "" {
			text = t
		}
		if h != "" {
			html = h
		}
	}
	return text, html
}
