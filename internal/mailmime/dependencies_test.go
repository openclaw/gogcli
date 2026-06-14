package mailmime

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

var errTestRead = errors.New("read failed")

func TestBuildRFC822RequiresRuntimeDependencies(t *testing.T) {
	opts := Options{
		From:    "sender@example.com",
		To:      []string{"recipient@example.com"},
		Subject: "subject",
		Body:    "body",
	}
	valid := Config{
		DateLocation: time.UTC,
		Now:          time.Now,
		Random:       bytes.NewReader(make([]byte, 16)),
	}

	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "date location", cfg: Config{Now: valid.Now, Random: valid.Random}, want: "date location is required"},
		{name: "clock", cfg: Config{DateLocation: valid.DateLocation, Random: valid.Random}, want: "clock is required"},
		{name: "random", cfg: Config{DateLocation: valid.DateLocation, Now: valid.Now}, want: "random source is required"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildRFC822(opts, test.cfg)
			if err == nil || err.Error() != test.want {
				t.Fatalf("BuildRFC822 error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildRFC822UsesSuppliedClockAndRandomSource(t *testing.T) {
	now := time.Date(2026, time.June, 13, 12, 34, 56, 0, time.UTC)

	raw, err := BuildRFC822(Options{
		From:     "sender@example.com",
		To:       []string{"recipient@example.com"},
		Subject:  "subject",
		Body:     "plain",
		BodyHTML: "<p>html</p>",
	}, Config{
		DateLocation: time.FixedZone("offset", 2*60*60),
		Now:          func() time.Time { return now },
		Random:       bytes.NewReader(make([]byte, 34)),
	})
	if err != nil {
		t.Fatalf("BuildRFC822: %v", err)
	}

	message := string(raw)
	if !strings.Contains(message, "Date: Sat, 13 Jun 2026 14:34:56 +0200") {
		t.Fatalf("missing supplied date: %q", message)
	}

	if !strings.Contains(message, "Message-ID: <AAAAAAAAAAAAAAAAAAAAAA@example.com>") {
		t.Fatalf("missing deterministic message ID: %q", message)
	}

	if !strings.Contains(message, `boundary="gogcli_AAAAAAAAAAAAAAAAAAAAAAAA"`) {
		t.Fatalf("missing deterministic boundary: %q", message)
	}
}

func TestPrepareAttachmentsRequiresAndUsesReader(t *testing.T) {
	attachments := []Attachment{{Path: "report.bin"}}
	if _, _, err := PrepareAttachments(attachments, nil); err == nil || err.Error() != "attachment file reader is required" {
		t.Fatalf("PrepareAttachments missing reader error = %v", err)
	}

	_, _, err := PrepareAttachments(attachments, func(path string) ([]byte, error) {
		if path != "report.bin" {
			t.Fatalf("path = %q", path)
		}

		return nil, errTestRead
	})
	if !errors.Is(err, errTestRead) {
		t.Fatalf("PrepareAttachments error = %v, want wrapped read error", err)
	}
}
