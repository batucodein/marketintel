package channel

import (
	"net"
	"strings"
)

// SuggestedSettings prefills the connect form for an email address. ProviderHint,
// when set, tells the UI to show provider-specific guidance (e.g. Gmail/Workspace
// needs IMAP enabled + an app password; Microsoft 365 may block IMAP).
type SuggestedSettings struct {
	IMAPHost     string `json:"imap_host"`
	IMAPPort     int    `json:"imap_port"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	Security     string `json:"security"`
	ProviderHint string `json:"provider_hint,omitempty"` // "gmail" | "outlook" | ""
}

// gmailSettings / outlookSettings / etc. are the canonical client settings for
// each big provider, reused by both the domain table and MX detection.
var (
	gmailSettings   = SuggestedSettings{"imap.gmail.com", 993, "smtp.gmail.com", 465, SecurityTLS, "gmail"}
	outlookSettings = SuggestedSettings{"outlook.office365.com", 993, "smtp.office365.com", 587, SecuritySTARTTLS, "outlook"}
	zohoSettings    = SuggestedSettings{"imap.zoho.com", 993, "smtp.zoho.com", 465, SecurityTLS, ""}
	yahooSettings   = SuggestedSettings{"imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 465, SecurityTLS, ""}
	icloudSettings  = SuggestedSettings{"imap.mail.me.com", 993, "smtp.mail.me.com", 587, SecuritySTARTTLS, ""}
	godaddySettings = SuggestedSettings{"imap.secureserver.net", 993, "smtpout.secureserver.net", 465, SecurityTLS, ""}
	titanSettings   = SuggestedSettings{"imap.titan.email", 993, "smtp.titan.email", 465, SecurityTLS, ""}
)

// providerTable maps well-known public mail domains directly to their settings.
var providerTable = map[string]SuggestedSettings{
	"gmail.com":      gmailSettings,
	"googlemail.com": gmailSettings,
	"outlook.com":    outlookSettings,
	"hotmail.com":    outlookSettings,
	"live.com":       outlookSettings,
	"office365.com":  outlookSettings,
	"zoho.com":       zohoSettings,
	"yahoo.com":      yahooSettings,
	"icloud.com":     icloudSettings,
	"me.com":         icloudSettings,
	"fastmail.com":   {"imap.fastmail.com", 993, "smtp.fastmail.com", 465, SecurityTLS, ""},
}

// SuggestEmailSettings returns IMAP/SMTP settings for an email address. Order:
//  1. exact match on a well-known public domain (gmail.com, …),
//  2. MX lookup → recognise the provider hosting a CUSTOM domain (e.g. a domain
//     on Google Workspace or GoDaddy — this is what makes "info@petravera.com"
//     resolve to imap.gmail.com),
//  3. fall back to a guess (imap./smtp.<domain>, implicit TLS).
//
// This only PREFILLS the form — the user can override any field, and the connect
// step tests the credentials before saving, so a wrong guess never persists.
func SuggestEmailSettings(email string) SuggestedSettings {
	domain := domainOf(email)
	if domain == "" {
		return SuggestedSettings{}
	}
	if s, ok := providerTable[domain]; ok {
		return s
	}
	if s, ok := detectByMX(domain); ok {
		return s
	}
	return SuggestedSettings{
		IMAPHost: "imap." + domain, IMAPPort: 993,
		SMTPHost: "smtp." + domain, SMTPPort: 465,
		Security: SecurityTLS,
	}
}

// detectByMX looks up the domain's MX records and maps a recognised mail provider
// to its IMAP/SMTP settings. ok=false when there are no MX records or the host
// isn't one we recognise (caller then falls back to a guess).
func detectByMX(domain string) (SuggestedSettings, bool) {
	mxs, err := net.LookupMX(domain)
	if err != nil || len(mxs) == 0 {
		return SuggestedSettings{}, false
	}
	var hosts string
	for _, mx := range mxs {
		hosts += strings.ToLower(mx.Host) + " "
	}
	switch {
	case strings.Contains(hosts, "google.com") || strings.Contains(hosts, "googlemail.com"):
		return gmailSettings, true
	case strings.Contains(hosts, "outlook.com") || strings.Contains(hosts, "office365") ||
		strings.Contains(hosts, "protection.outlook") || strings.Contains(hosts, "microsoft"):
		return outlookSettings, true
	case strings.Contains(hosts, "secureserver.net"):
		return godaddySettings, true
	case strings.Contains(hosts, "titan.email"):
		return titanSettings, true
	case strings.Contains(hosts, "zoho"):
		return zohoSettings, true
	case strings.Contains(hosts, "yahoodns") || strings.Contains(hosts, "yahoo"):
		return yahooSettings, true
	case strings.Contains(hosts, "icloud") || strings.Contains(hosts, "apple"):
		return icloudSettings, true
	}
	return SuggestedSettings{}, false
}

func domainOf(email string) string {
	d := strings.ToLower(strings.TrimSpace(email))
	if i := strings.LastIndex(d, "@"); i >= 0 {
		d = d[i+1:]
	}
	return d
}
