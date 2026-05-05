package datasource

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebsiteScraper extracts email addresses and company info from websites.
type WebsiteScraper struct {
	client       *http.Client
	jina         *JinaReader
	userAgents   []string
	tier2Bytes   int    // fall back to Jina when Tier 1 returns less plain text than this
}

// defaultUserAgents are modern desktop UA strings rotated per-request to
// reduce 403s from naive bot blockers. Order is randomised per scraper
// instance.
var defaultUserAgents = []string{
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_2) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:128.0) Gecko/20100101 Firefox/128.0",
}

func NewWebsiteScraper() *WebsiteScraper {
	return &WebsiteScraper{
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		jina:       NewJinaReader(""),
		userAgents: defaultUserAgents,
		tier2Bytes: 500,
	}
}

// SetTier2 wires the Jina fallback. Pass an empty baseURL to use the
// public free endpoint, or override for self-hosted Jina.
func (s *WebsiteScraper) SetTier2(jinaBaseURL string, thresholdBytes int) {
	s.jina = NewJinaReader(jinaBaseURL)
	if thresholdBytes > 0 {
		s.tier2Bytes = thresholdBytes
	}
}

// ScrapeResult contains everything extracted from a company website.
type ScrapeResult struct {
	Email       string            // best email for B2B outreach
	AllEmails   []string          // all valid emails found (ranked by usefulness)
	Phones      []string          // all phone numbers found on the website
	Description string            // company description extracted from about/home page
	SocialLinks map[string]string // linkedin, facebook, twitter, instagram URLs
	WhatsApp    string            // WhatsApp link/number if found
	Products    []string          // product/service keywords extracted
	HasContactForm bool           // whether a contact form was detected
}

// emailRe matches standard email addresses.
var emailRe = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

// socialRe extracts social media profile URLs.
var socialPatterns = map[string]*regexp.Regexp{
	"linkedin":  regexp.MustCompile(`https?://(?:www\.)?linkedin\.com/(?:company|in)/[a-zA-Z0-9_\-/]+`),
	"facebook":  regexp.MustCompile(`https?://(?:www\.)?facebook\.com/[a-zA-Z0-9._\-]+`),
	"twitter":   regexp.MustCompile(`https?://(?:www\.)?(?:twitter|x)\.com/[a-zA-Z0-9_]+`),
	"instagram": regexp.MustCompile(`https?://(?:www\.)?instagram\.com/[a-zA-Z0-9._]+`),
}

// phoneRe matches international phone numbers in various formats.
var phoneRe = regexp.MustCompile(`(?:(?:\+|00)\d{1,3}[\s\-.]?)?\(?\d{2,4}\)?[\s\-.]?\d{3,4}[\s\-.]?\d{2,4}(?:[\s\-.]?\d{2,4})?`)

// whatsappRe matches WhatsApp links.
var whatsappRe = regexp.MustCompile(`https?://(?:wa\.me|api\.whatsapp\.com/send\?phone=)(\d+)`)

// telRe matches tel: links in HTML (more reliable than text-pattern matching).
var telRe = regexp.MustCompile(`(?i)(?:href|content)=["']tel:([+\d\s\-().]+)["']`)

// contactFormRe detects contact forms.
var contactFormRe = regexp.MustCompile(`(?i)<form[^>]*(?:contact|inquiry|enquiry|message|quote|request)[^>]*>|<input[^>]*(?:name|id)=["'](?:email|phone|message|inquiry)["']`)

// HTML tag stripping regex.
var (
	stripBlockRes = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`),
		regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`),
		regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`),
		regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`),
		regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`),
		regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`),
	}
	tagRe        = regexp.MustCompile(`<[^>]+>`)
	whitespaceRe = regexp.MustCompile(`[ \t]+`)
	blankLinesRe = regexp.MustCompile(`\n{3,}`)
)

// excludePatterns filters out non-human emails.
var excludePatterns = []string{
	"@sentry.", "@wixpress.", "@googleapis.", "@example.",
	"@sentry-next.", "@emailprotection.",
	".png", ".jpg", ".gif", ".svg", ".webp", ".css", ".js",
	"noreply@", "no-reply@", "mailer-daemon@",
}

// Scrape fetches a website and extracts email, description, social links, phones, and more.
func (s *WebsiteScraper) Scrape(ctx context.Context, websiteURL string) (*ScrapeResult, error) {
	baseURL := normalizeURL(websiteURL)
	if baseURL == "" {
		return nil, fmt.Errorf("invalid URL")
	}

	result := &ScrapeResult{
		SocialLinks: make(map[string]string),
	}

	// Pages to fetch: homepage first, then about, then contact, then products
	pagePaths := []struct {
		path       string
		isAbout    bool
		isContact  bool
		isProducts bool
	}{
		{"", false, false, false},
		{"/about", true, false, false},
		{"/about-us", true, false, false},
		{"/contact", false, true, false},
		{"/contact-us", false, true, false},
		{"/products", false, false, true},
		{"/services", false, false, true},
	}

	var allEmails []string
	emailSeen := make(map[string]bool)
	phoneSeen := make(map[string]bool)
	var bestAboutText string
	var productText string

	for _, pp := range pagePaths {
		url := baseURL + pp.path
		html, err := s.fetchPage(ctx, url)
		if err != nil {
			slog.Debug("websitescraper: fetch failed", "url", url, "error", err)
			continue
		}

		// Extract emails from decoded HTML
		decoded := strings.ReplaceAll(html, "&#64;", "@")
		decoded = strings.ReplaceAll(decoded, "%40", "@")
		for _, e := range emailRe.FindAllString(decoded, 50) {
			lower := strings.ToLower(e)
			if !emailSeen[lower] && !isExcluded(lower) {
				emailSeen[lower] = true
				allEmails = append(allEmails, lower)
			}
		}

		// Extract phone numbers from tel: links (most reliable)
		for _, match := range telRe.FindAllStringSubmatch(html, 20) {
			if len(match) > 1 {
				phone := normalizePhone(match[1])
				if phone != "" && !phoneSeen[phone] {
					phoneSeen[phone] = true
					result.Phones = append(result.Phones, phone)
				}
			}
		}

		// Extract phone numbers from visible text on contact pages
		if pp.isContact {
			text := htmlToText(html)
			for _, match := range phoneRe.FindAllString(text, 20) {
				phone := normalizePhone(match)
				if phone != "" && len(phone) >= 7 && !phoneSeen[phone] {
					phoneSeen[phone] = true
					result.Phones = append(result.Phones, phone)
				}
			}
		}

		// Extract WhatsApp link
		if result.WhatsApp == "" {
			if matches := whatsappRe.FindStringSubmatch(html); len(matches) > 1 {
				result.WhatsApp = "+" + matches[1]
			}
		}

		// Extract social links
		for name, re := range socialPatterns {
			if _, ok := result.SocialLinks[name]; ok {
				continue
			}
			if m := re.FindString(html); m != "" {
				result.SocialLinks[name] = m
			}
		}

		// Detect contact form
		if !result.HasContactForm && contactFormRe.MatchString(html) {
			result.HasContactForm = true
		}

		// Extract text from about pages for description
		if pp.isAbout && bestAboutText == "" {
			text := htmlToText(html)
			if len(text) > 100 {
				bestAboutText = text
			}
		}

		// Extract product keywords from product/service pages
		if pp.isProducts && productText == "" {
			text := htmlToText(html)
			if len(text) > 50 {
				productText = text
			}
		}
	}

	// If no about text found, use homepage text as fallback
	if bestAboutText == "" {
		html, err := s.fetchPage(ctx, baseURL)
		if err == nil {
			bestAboutText = htmlToText(html)
		}
	}

	// Rank and set emails
	if len(allEmails) > 0 {
		result.Email = pickBestEmail(allEmails, baseURL)
		result.AllEmails = rankEmails(allEmails, baseURL)
	}

	// Build description from about text
	if bestAboutText != "" {
		result.Description = buildDescription(bestAboutText)
	}

	// Extract product keywords
	if productText != "" {
		result.Products = extractProductKeywords(productText)
	}

	return result, nil
}

func (s *WebsiteScraper) fetchPage(ctx context.Context, url string) (string, error) {
	body, err := s.fetchTier1(ctx, url)
	if err == nil && plainTextLen(body) >= s.tier2Bytes {
		return body, nil
	}

	// Tier 2 fallback — Jina Reader. Triggered on 403/blocked AND on
	// thin responses where JS-rendering is likely required.
	if s.jina != nil {
		md, jErr := s.jina.Fetch(ctx, url)
		if jErr == nil && len(md) > 0 {
			return md, nil
		}
	}

	if err != nil {
		return body, err
	}
	return body, nil
}

// fetchTier1 is the original net/http path with rotated User-Agents.
func (s *WebsiteScraper) fetchTier1(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", s.pickUA())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.9")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// pickUA rotates through the configured user-agent list pseudo-randomly
// using time-based selection (cheap, no shared state to lock).
func (s *WebsiteScraper) pickUA() string {
	if len(s.userAgents) == 0 {
		return "Mozilla/5.0 (compatible; MarketIntel/1.0)"
	}
	idx := int(time.Now().UnixNano()/int64(time.Millisecond)) % len(s.userAgents)
	return s.userAgents[idx]
}

// plainTextLen estimates the readable text size of a body so we can
// decide whether it's worth piping into the AI extractor or whether
// we should fall through to Jina. HTML pages with mostly scripts get a
// low score even if they're large in bytes.
func plainTextLen(body string) int {
	if body == "" {
		return 0
	}
	stripped := htmlToText(body)
	return len(stripped)
}

// htmlToText strips HTML tags and returns clean text.
func htmlToText(html string) string {
	// Remove script, style, noscript, svg, head, nav, footer blocks
	text := html
	for _, re := range stripBlockRes {
		text = re.ReplaceAllString(text, "")
	}
	// Remove HTML tags
	text = tagRe.ReplaceAllString(text, " ")
	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	// Normalize whitespace
	text = whitespaceRe.ReplaceAllString(text, " ")
	text = blankLinesRe.ReplaceAllString(text, "\n")
	return strings.TrimSpace(text)
}

// buildDescription takes raw about-page text and extracts a useful company description.
// Truncates to ~500 chars at sentence boundary.
func buildDescription(text string) string {
	// Split into lines, filter out very short lines (nav items, buttons)
	lines := strings.Split(text, "\n")
	var meaningful []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 30 {
			continue // skip nav items, single words, etc.
		}
		meaningful = append(meaningful, line)
	}

	if len(meaningful) == 0 {
		return ""
	}

	// Join and truncate
	joined := strings.Join(meaningful, " ")
	if len(joined) > 500 {
		// Cut at last sentence boundary before 500
		cut := joined[:500]
		if lastDot := strings.LastIndex(cut, "."); lastDot > 200 {
			cut = cut[:lastDot+1]
		}
		joined = cut
	}
	return strings.TrimSpace(joined)
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return ""
	}
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = "https://" + raw
	}
	return raw
}

func isExcluded(email string) bool {
	for _, pattern := range excludePatterns {
		if strings.Contains(email, pattern) {
			return true
		}
	}
	return false
}

// pickBestEmail ranks emails by usefulness for B2B outreach.
func pickBestEmail(emails []string, baseURL string) string {
	domain := extractDomain(baseURL)

	type scored struct {
		email string
		score int
	}
	var scoredEmails []scored

	for _, e := range emails {
		s := 0
		if domain != "" && strings.HasSuffix(e, "@"+domain) {
			s += 100
		}
		prefix := strings.Split(e, "@")[0]
		switch {
		case prefix == "sales":
			s += 50
		case prefix == "info":
			s += 40
		case prefix == "contact":
			s += 35
		case prefix == "hello":
			s += 30
		case prefix == "office":
			s += 25
		case prefix == "enquiry" || prefix == "enquiries":
			s += 25
		case prefix == "admin":
			s += 10
		default:
			s += 5
		}
		scoredEmails = append(scoredEmails, scored{email: e, score: s})
	}

	best := scoredEmails[0]
	for _, se := range scoredEmails[1:] {
		if se.score > best.score {
			best = se
		}
	}
	return best.email
}

func extractDomain(url string) string {
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "www.")
	if i := strings.Index(url, "/"); i > 0 {
		url = url[:i]
	}
	return strings.ToLower(url)
}

// normalizePhone cleans a phone string to a consistent format.
func normalizePhone(raw string) string {
	// Keep only digits, +, and leading +
	var sb strings.Builder
	for i, ch := range raw {
		if ch == '+' && i == 0 {
			sb.WriteRune(ch)
		} else if ch >= '0' && ch <= '9' {
			sb.WriteRune(ch)
		}
	}
	phone := sb.String()
	// Filter out numbers that are too short (likely not phones) or too long
	digits := strings.TrimPrefix(phone, "+")
	if len(digits) < 7 || len(digits) > 15 {
		return ""
	}
	return phone
}

// rankEmails returns all emails sorted by B2B usefulness (best first).
func rankEmails(emails []string, baseURL string) []string {
	domain := extractDomain(baseURL)
	type scored struct {
		email string
		score int
	}
	var list []scored
	for _, e := range emails {
		s := 0
		if domain != "" && strings.HasSuffix(e, "@"+domain) {
			s += 100
		}
		prefix := strings.Split(e, "@")[0]
		switch {
		case prefix == "sales":
			s += 50
		case prefix == "info":
			s += 40
		case prefix == "contact":
			s += 35
		case prefix == "hello":
			s += 30
		case prefix == "office":
			s += 25
		case prefix == "enquiry" || prefix == "enquiries":
			s += 25
		case prefix == "export" || prefix == "import" || prefix == "trade":
			s += 45
		case prefix == "purchasing" || prefix == "procurement":
			s += 45
		case prefix == "admin":
			s += 10
		default:
			s += 5
		}
		list = append(list, scored{email: e, score: s})
	}
	// Sort descending by score
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if list[j].score > list[i].score {
				list[i], list[j] = list[j], list[i]
			}
		}
	}
	result := make([]string, len(list))
	for i, s := range list {
		result[i] = s.email
	}
	return result
}

// extractProductKeywords pulls product/service terms from page text.
func extractProductKeywords(text string) []string {
	// Split into lines and extract short, meaningful phrases
	lines := strings.Split(text, "\n")
	seen := make(map[string]bool)
	var keywords []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Product keywords are typically 2-60 chars, not full paragraphs
		if len(line) < 3 || len(line) > 60 {
			continue
		}
		// Skip lines that look like navigation/UI text
		lower := strings.ToLower(line)
		if strings.ContainsAny(lower, "©@{}") {
			continue
		}
		skipWords := []string{"home", "about", "contact", "login", "sign", "menu", "search", "privacy", "cookie", "terms"}
		skip := false
		for _, w := range skipWords {
			if lower == w || strings.HasPrefix(lower, w+" ") {
				skip = true
				break
			}
		}
		if skip || seen[lower] {
			continue
		}
		seen[lower] = true
		keywords = append(keywords, line)
		if len(keywords) >= 20 {
			break
		}
	}
	return keywords
}

// VerifyEmail does a basic SMTP-level check to see if an email address is deliverable.
// It connects to the MX server and checks if RCPT TO is accepted. Does NOT send any email.
func VerifyEmail(ctx context.Context, email string) bool {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return false
	}
	domain := parts[1]

	// Look up MX records
	mxRecords, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil || len(mxRecords) == 0 {
		// No MX records — try A record as fallback
		_, err := net.DefaultResolver.LookupHost(ctx, domain)
		return err == nil // at least the domain exists
	}

	// Try the highest-priority MX
	mx := mxRecords[0].Host
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", mx+":25")
	if err != nil {
		return true // can't connect to SMTP, but MX exists — assume valid (many block port 25)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	reader := func() (string, error) {
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	}

	write := func(cmd string) error {
		_, err := conn.Write([]byte(cmd + "\r\n"))
		return err
	}

	// Read banner
	if _, err := reader(); err != nil {
		return true // connection issue, assume valid
	}

	// EHLO
	if err := write("EHLO marketintel.local"); err != nil {
		return true
	}
	if _, err := reader(); err != nil {
		return true
	}

	// MAIL FROM
	if err := write("MAIL FROM:<verify@marketintel.local>"); err != nil {
		return true
	}
	if _, err := reader(); err != nil {
		return true
	}

	// RCPT TO — this is the actual check
	if err := write("RCPT TO:<" + email + ">"); err != nil {
		return true
	}
	resp, err := reader()
	if err != nil {
		return true
	}

	// Quit politely
	_ = write("QUIT")

	// 250 = accepted, 251 = forwarded — both mean valid
	// 550, 551, 552, 553 = rejected
	return strings.HasPrefix(resp, "2")
}
