package gmail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"

	"github.com/batuhan/marketintel/internal/outreach/channel"
)

func TestBuildRFC5322_NoAttachment(t *testing.T) {
	out, err := buildRFC5322(channel.SendRequest{
		To:       "buyer@example.com",
		Subject:  "Hello",
		BodyText: "Hi there",
	}, "me@example.com")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	body, _ := io.ReadAll(msg.Body)
	if !strings.Contains(string(body), "Hi there") {
		t.Errorf("body missing, got %q", string(body))
	}
	if got := msg.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Errorf("expected text/plain, got %q", got)
	}
}

func TestBuildRFC5322_WithAttachment(t *testing.T) {
	pdf := []byte("%PDF-1.4\n%fake pdf bytes\n")
	out, err := buildRFC5322(channel.SendRequest{
		To:       "buyer@example.com",
		Subject:  "With catalog",
		BodyText: "See attached.",
		Attachments: []channel.Attachment{
			{Filename: "catalog.pdf", MimeType: "application/pdf", Data: pdf},
		},
	}, "me@example.com")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	msg, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("parse outer: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parse content-type: %v", err)
	}
	if mediaType != "multipart/mixed" {
		t.Fatalf("expected multipart/mixed, got %s", mediaType)
	}

	mr := multipart.NewReader(msg.Body, params["boundary"])
	var parts []string
	var attachmentData []byte
	var attachmentName string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		ct := p.Header.Get("Content-Type")
		parts = append(parts, ct)
		if strings.HasPrefix(ct, "application/pdf") {
			attachmentName = p.FileName()
			body, err := io.ReadAll(p)
			if err != nil {
				t.Fatalf("read attachment: %v", err)
			}
			// Decoder reads base64 transparently when CTE: base64 set.
			attachmentData = body
		}
	}
	if len(parts) != 2 {
		t.Fatalf("expected 2 parts, got %d: %v", len(parts), parts)
	}
	if attachmentName != "catalog.pdf" {
		t.Errorf("attachment filename mismatch, got %q", attachmentName)
	}
	if len(attachmentData) == 0 {
		t.Errorf("attachment body empty")
	}
	// Attachment body comes back as base64 text (we didn't decode). Confirm it decodes to original.
	// The multipart reader doesn't auto-decode CTE base64.
	if !bytes.Contains(attachmentData, []byte("JVBERi0xLjQ")) {
		t.Errorf("attachment body doesn't contain expected base64 of PDF header, got %q", string(attachmentData[:min(40, len(attachmentData))]))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
