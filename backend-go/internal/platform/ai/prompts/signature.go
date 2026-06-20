package prompts

import "strings"

// WithSignature guarantees the brand's signature appears exactly once at the
// end of an AI-drafted body. The model is inconsistent about reproducing the
// signature block, so we enforce it deterministically: if the signature text
// isn't already present, append it. No-op when signature is empty.
func WithSignature(body, signature string) string {
	sig := strings.TrimSpace(signature)
	if sig == "" {
		return body
	}
	if strings.Contains(body, sig) {
		return body
	}
	// Also skip if the first signature line is already there (model may have
	// reproduced it with minor trailing differences).
	if first := firstLine(sig); first != "" && strings.Contains(body, first) {
		return body
	}
	return strings.TrimRight(body, "\n") + "\n\n" + sig
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
