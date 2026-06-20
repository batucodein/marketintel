package prompts

import (
	"regexp"
	"strings"
)

// GuardViolation describes one specific reason a draft failed validation.
// Surfaced in logs so we can tune the prompts over time.
type GuardViolation struct {
	Kind   string // "catalog_question", "forbidden_phrase", "length_cap", "exclamation_in_cold"
	Detail string // e.g. the exact forbidden phrase found
}

// DraftMode is the type of email being validated — controls which rules
// apply (e.g. exclamation marks are forbidden in cold emails but allowed
// in replies).
type DraftMode int

const (
	DraftModeCold DraftMode = iota
	DraftModeReply
	DraftModeFollowup
)

// ValidateOutreachBody runs the post-generation guard against an AI
// draft body. Returns the list of violations found (empty = clean).
// Callers retry once when violations are non-empty.
//
// hasCatalog only matters for catalog-related violation checks — when
// false we don't care whether the body mentions a catalog.
func ValidateOutreachBody(body string, hasCatalog bool, mode DraftMode) []GuardViolation {
	var out []GuardViolation
	lower := strings.ToLower(body)

	// 1. Catalog-permission hedging when the catalog is already attached.
	if hasCatalog {
		for _, pat := range catalogHedgePatterns {
			if pat.MatchString(lower) {
				out = append(out, GuardViolation{
					Kind:   "catalog_question",
					Detail: pat.String(),
				})
				break // one hit is enough
			}
		}
	}

	// 2. Forbidden filler phrases.
	for _, p := range forbiddenPhrases {
		if strings.Contains(lower, p) {
			// "circle back" / "touch base" are allowed in warm follow-ups.
			if (p == "circle back" || p == "touch base") && mode == DraftModeFollowup {
				continue
			}
			out = append(out, GuardViolation{
				Kind:   "forbidden_phrase",
				Detail: p,
			})
		}
	}

	// 3. Exclamation marks in cold emails.
	if mode == DraftModeCold && strings.Contains(body, "!") {
		out = append(out, GuardViolation{
			Kind:   "exclamation_in_cold",
			Detail: "exclamation mark in cold email body",
		})
	}

	// 4. Length cap. Word count, body only.
	wc := wordCount(body)
	cap := lengthCap(mode)
	if wc > cap {
		out = append(out, GuardViolation{
			Kind:   "length_cap",
			Detail: formatLengthDetail(wc, cap),
		})
	}

	return out
}

func lengthCap(mode DraftMode) int {
	switch mode {
	case DraftModeCold:
		return 140
	case DraftModeReply:
		return 110
	case DraftModeFollowup:
		return 90
	}
	return 140
}

func formatLengthDetail(wc, cap int) string {
	return "body is " + itoa(wc) + " words, cap is " + itoa(cap)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 6)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

func wordCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Fields(s))
}

// catalogHedgePatterns matches the typical AI-asks-permission-for-catalog
// phrasings. Compiled once at package init for cheap repeated matching.
var catalogHedgePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)would (it )?(be useful|it make sense|you like) .{0,30}\bcatalog`),
	regexp.MustCompile(`(?i)\bshould i (send|share|forward) .{0,30}\bcatalog`),
	regexp.MustCompile(`(?i)let me know if .{0,30}\bcatalog`),
	regexp.MustCompile(`(?i)\bi can (send|share|forward) .{0,30}\bcatalog`),
	regexp.MustCompile(`(?i)happy to (send|share|forward) .{0,30}\bcatalog`),
	regexp.MustCompile(`(?i)if you('|`+`re)? interested, .{0,30}\bcatalog`),
}

// forbiddenPhrases — lower-cased substrings whose presence is an instant
// rewrite. Kept narrow on purpose: these are the worst offenders that
// nearly every recipient will recognise as AI-written.
var forbiddenPhrases = []string{
	"hope this email finds you well",
	"hope this finds you well",
	"i came across your company",
	"i wanted to reach out",
	"perfect synergy",
	"leverage our partnership",
	"feel free to",
	"let me know if you have any questions",
	"looking forward to hearing from you",
	"circle back",
	"touch base",
}
