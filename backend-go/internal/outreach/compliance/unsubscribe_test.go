package compliance

import (
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
