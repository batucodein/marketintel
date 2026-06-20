package poller

import "testing"

func TestParseEmailAddress(t *testing.T) {
	cases := map[string]string{
		"Batuhan Özalhan <batu@petravera.com>": "batu@petravera.com",
		"batu@petravera.com":                   "batu@petravera.com",
		"<batu@petravera.com>":                 "batu@petravera.com",
		"BATU@PETRAVERA.COM":                   "batu@petravera.com",
		`"Sales, Acme" <sales@acme.co>`:        "sales@acme.co",
		"":                                     "",
		"not-an-email":                         "",
	}
	for in, want := range cases {
		if got := parseEmailAddress(in); got != want {
			t.Errorf("parseEmailAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
