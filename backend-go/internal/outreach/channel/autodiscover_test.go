package channel

import "testing"

func TestSuggestEmailSettings(t *testing.T) {
	// Known public domains hit the table directly (no network).
	g := SuggestEmailSettings("sales@gmail.com")
	if g.IMAPHost != "imap.gmail.com" || g.SMTPHost != "smtp.gmail.com" || g.ProviderHint != "gmail" {
		t.Errorf("gmail suggestion wrong: %+v", g)
	}
	o := SuggestEmailSettings("rep@outlook.com")
	if o.ProviderHint != "outlook" || o.Security != SecuritySTARTTLS {
		t.Errorf("outlook suggestion wrong: %+v", o)
	}
	// Case-insensitive.
	if SuggestEmailSettings("X@GMAIL.COM").ProviderHint != "gmail" {
		t.Error("should be case-insensitive on domain")
	}
	// A reserved .invalid domain has no MX → falls back to the guess (offline,
	// deterministic). Custom domains on a real provider are resolved via MX and
	// covered by integration, since that needs DNS.
	c := SuggestEmailSettings("info@petravera.invalid")
	if c.IMAPHost != "imap.petravera.invalid" || c.SMTPHost != "smtp.petravera.invalid" {
		t.Errorf("guess fallback wrong: %+v", c)
	}
	if c.IMAPPort != 993 || c.SMTPPort != 465 || c.Security != SecurityTLS || c.ProviderHint != "" {
		t.Errorf("guess defaults wrong: %+v", c)
	}
}
