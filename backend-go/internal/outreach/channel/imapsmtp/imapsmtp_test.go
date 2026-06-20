package imapsmtp

import (
	"testing"

	"github.com/batuhan/marketintel/internal/outreach/channel"
	"github.com/batuhan/marketintel/internal/platform/crypto"
)

func TestConfigRoundTrip(t *testing.T) {
	cipher, err := crypto.NewFromKey([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	cfg := channel.SMTPConfig{
		IMAPHost: "imap.petravera.com", IMAPPort: 993,
		SMTPHost: "smtp.petravera.com", SMTPPort: 465,
		Username: "batu@petravera.com", Password: "s3cr3t-app-pw",
		Security: channel.SecurityTLS,
	}
	enc, err := EncodeConfig(cipher, cfg)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// The stored blob must NOT contain the plaintext password.
	if containsPlaintext(string(enc), cfg.Password) {
		t.Fatal("encrypted config leaked the plaintext password")
	}
	got, err := DecodeConfig(cipher, enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != cfg {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, cfg)
	}
}

func containsPlaintext(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
