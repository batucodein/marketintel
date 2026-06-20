// Package compliance handles outbound-email regulatory plumbing
// (CAN-SPAM, GDPR): unsubscribe links, suppression checks, and the public
// landing page recipients hit when they click the link in an email footer.
//
// One-click unsubscribe is HMAC-signed with a per-channel secret so the
// recipient does not need an account. We verify the signature, mark the
// contact suppressed, and never email them again.
package compliance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

// Token is the value embedded in the unsubscribe URL. Format:
//
//	v1.<contact_id>.<campaign_id>.<issued_unix>.<base64-hmac>
//
// The HMAC covers everything before the trailing ".<sig>" using the channel's
// unsubscribe_secret. Tokens never expire — recipients keep their right to
// unsubscribe forever. The issued_unix field is informational only.
type Token struct {
	ContactID  uuid.UUID
	CampaignID uuid.UUID
	IssuedAt   time.Time
}

// GenerateToken creates a signed token for the given contact + campaign.
// channel.UnsubscribeSecret must be non-empty (column is NOT NULL since 0012).
func GenerateToken(channel domain.UserChannel, contactID, campaignID uuid.UUID) (string, error) {
	if len(channel.UnsubscribeSecret) == 0 {
		return "", errors.New("channel missing unsubscribe secret")
	}
	body := fmt.Sprintf("v1.%s.%s.%d", contactID, campaignID, time.Now().Unix())
	mac := hmac.New(sha256.New, channel.UnsubscribeSecret)
	mac.Write([]byte(body))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return body + "." + sig, nil
}

// VerifyToken checks the signature and returns the parsed fields.
// Returns ok=false on any tampering or unknown format.
func VerifyToken(channel domain.UserChannel, token string) (Token, bool) {
	if len(channel.UnsubscribeSecret) == 0 {
		return Token{}, false
	}
	// Split off the last dot-separated chunk (the signature).
	idx := strings.LastIndex(token, ".")
	if idx < 0 {
		return Token{}, false
	}
	body, providedSig := token[:idx], token[idx+1:]
	mac := hmac.New(sha256.New, channel.UnsubscribeSecret)
	mac.Write([]byte(body))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(providedSig)) {
		return Token{}, false
	}
	parts := strings.Split(body, ".")
	if len(parts) != 4 || parts[0] != "v1" {
		return Token{}, false
	}
	cid, err := uuid.Parse(parts[1])
	if err != nil {
		return Token{}, false
	}
	camp, err := uuid.Parse(parts[2])
	if err != nil {
		return Token{}, false
	}
	var issued int64
	if _, err := fmt.Sscanf(parts[3], "%d", &issued); err != nil {
		return Token{}, false
	}
	return Token{ContactID: cid, CampaignID: camp, IssuedAt: time.Unix(issued, 0)}, true
}

// BuildUnsubscribeURL returns the user-clickable URL embedded in email
// footers. publicAPIURL is the externally-reachable base URL of the backend
// (e.g. https://marketintel-...run.app).
func BuildUnsubscribeURL(publicAPIURL string, channel domain.UserChannel, contactID, campaignID uuid.UUID) (string, error) {
	tok, err := GenerateToken(channel, contactID, campaignID)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(publicAPIURL, "/") + "/unsubscribe?token=" + tok, nil
}

// IsSuppressed reports whether the contact has unsubscribed.
// Senders MUST call this immediately before dispatching any campaign or
// sequence message — never trust a stale in-memory contact.
func IsSuppressed(c *domain.Contact) bool {
	return c != nil && c.UnsubscribedAt != nil
}

// StripQuotedReply returns only the NEW text of an inbound reply: everything
// from the first quoted-history marker onward is dropped. Critical because our
// own outbound footer contains the word "Unsubscribe" — scanning a reply that
// quotes it would suppress every warm lead (and contaminate AI classification).
// Markers handled: ">"-quoted lines, "On ... wrote:" attribution lines, the
// "---" footer separator we append, Outlook's original-message separators, and
// a bare "Unsubscribe:" footer line.
func StripQuotedReply(body string) string {
	if body == "" {
		return ""
	}
	lines := strings.Split(body, "\n")
	var kept []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, ">"):
			return strings.TrimSpace(strings.Join(kept, "\n"))
		case strings.HasPrefix(t, "On ") && strings.HasSuffix(t, "wrote:"):
			return strings.TrimSpace(strings.Join(kept, "\n"))
		case t == "---" || t == "--" || strings.HasPrefix(t, "-----Original Message-----") ||
			strings.HasPrefix(t, "________________________________"):
			return strings.TrimSpace(strings.Join(kept, "\n"))
		case strings.HasPrefix(t, "Unsubscribe: http"):
			return strings.TrimSpace(strings.Join(kept, "\n"))
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// LooksLikeOptOut returns true when the body of an inbound message contains
// language we should treat as an unsubscribe request. Conservative — false
// positives lock a contact out of further outreach. Callers MUST pass the
// stripped new-text portion (StripQuotedReply), never the raw body.
func LooksLikeOptOut(body string) bool {
	if body == "" {
		return false
	}
	low := strings.ToLower(body)
	for _, kw := range []string{
		"unsubscribe",
		"please remove me",
		"remove me from",
		"stop emailing",
		"do not contact",
		"opt out",
		"opt-out",
	} {
		if strings.Contains(low, kw) {
			return true
		}
	}
	return false
}

// FooterText returns the plain-text footer block to append to outbound bulk
// emails. Composed from the sender's physical address (CAN-SPAM requirement)
// and the signed unsubscribe URL.
func FooterText(physicalAddress, unsubscribeURL string) string {
	if unsubscribeURL == "" {
		return ""
	}
	addr := strings.TrimSpace(physicalAddress)
	if addr == "" {
		addr = "(physical address not configured)"
	}
	return "\n\n---\n" +
		"You are receiving this email because we identified your business via public trade data.\n" +
		addr + "\n" +
		"Unsubscribe: " + unsubscribeURL
}
