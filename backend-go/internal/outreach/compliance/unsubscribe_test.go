package compliance

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/batuhan/marketintel/internal/domain"
)

func TestTokenRoundTrip(t *testing.T) {
	ch := domain.UserChannel{
		ID:                uuid.New(),
		UnsubscribeSecret: []byte("0123456789abcdef0123456789abcdef"),
	}
	contact := uuid.New()
	camp := uuid.New()

	tok, err := GenerateToken(ch, contact, camp)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	got, ok := VerifyToken(ch, tok)
	if !ok {
		t.Fatalf("verify failed for valid token")
	}
	if got.ContactID != contact || got.CampaignID != camp {
		t.Errorf("decoded ids mismatch: got %v %v want %v %v", got.ContactID, got.CampaignID, contact, camp)
	}
}

func TestTamperedTokenRejected(t *testing.T) {
	ch := domain.UserChannel{
		UnsubscribeSecret: []byte("0123456789abcdef0123456789abcdef"),
	}
	tok, _ := GenerateToken(ch, uuid.New(), uuid.New())
	if _, ok := VerifyToken(ch, tok+"x"); ok {
		t.Error("tampered token accepted")
	}
	other := domain.UserChannel{
		UnsubscribeSecret: []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
	}
	if _, ok := VerifyToken(other, tok); ok {
		t.Error("token accepted under wrong secret")
	}
}

func TestLooksLikeOptOut(t *testing.T) {
	cases := map[string]bool{
		"please unsubscribe me from this list": true,
		"Hey, please remove me from your list":  true,
		"STOP EMAILING me":                       true,
		"Sounds great, let's chat":               false,
		"":                                       false,
	}
	for body, want := range cases {
		if got := LooksLikeOptOut(body); got != want {
			t.Errorf("LooksLikeOptOut(%q) = %v, want %v", body, got, want)
		}
	}
}

func TestStripQuotedReply(t *testing.T) {
	footer := FooterText("123 Trade St, Izmir", "https://api.example.com/unsubscribe?token=abc")

	// The critical regression: a warm reply quoting our own footer must NOT
	// look like an opt-out, and classification must only see the new text.
	warmQuoted := "Yes! Please send me your pricing.\n\nOn Tue, Jun 9, 2026 at 4:01 PM Selin <selin@brand.com> wrote:\n> Hello,\n> We export marble." +
		"\n> " + strings.ReplaceAll(footer, "\n", "\n> ")
	if got := StripQuotedReply(warmQuoted); got != "Yes! Please send me your pricing." {
		t.Errorf("StripQuotedReply(warmQuoted) = %q", got)
	}
	if LooksLikeOptOut(StripQuotedReply(warmQuoted)) {
		t.Error("warm reply quoting our footer must not be treated as opt-out")
	}

	// Unquoted footer leak (some clients in-line the original without '>').
	leak := "Looks interesting, call me.\n---\nYou are receiving this email because...\nUnsubscribe: https://x"
	if got := StripQuotedReply(leak); got != "Looks interesting, call me." {
		t.Errorf("StripQuotedReply(leak) = %q", got)
	}

	// A genuine opt-out in the new text still triggers.
	if !LooksLikeOptOut(StripQuotedReply("please unsubscribe me\n\nOn Mon someone wrote:\n> old text")) {
		t.Error("genuine opt-out in new text must still trigger")
	}

	// Outlook-style separator.
	if got := StripQuotedReply("Thanks.\n________________________________\nFrom: x@y.com"); got != "Thanks." {
		t.Errorf("outlook strip = %q", got)
	}

	// Pure-quote body strips to empty (treated as no new text).
	if got := StripQuotedReply("> everything quoted\n> nothing new"); got != "" {
		t.Errorf("pure quote = %q", got)
	}

	// No markers → unchanged.
	if got := StripQuotedReply("Plain reply with no quoting."); got != "Plain reply with no quoting." {
		t.Errorf("plain = %q", got)
	}
}
