package errfmt

import (
	"errors"
	"strings"
	"testing"

	"github.com/99designs/keyring"
	"github.com/alecthomas/kong"
	ggoogleapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
	gogapi "github.com/steipete/gogcli/internal/googleapi"
)

var errNope = errors.New("nope")

func TestFormat_Nil(t *testing.T) {
	if got := Format(nil); got != "" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_AuthRequired(t *testing.T) {
	err := &gogapi.AuthRequiredError{Service: "gmail", Email: "a@b.com", Cause: keyring.ErrKeyNotFound}
	got := Format(err)

	if got == "" {
		t.Fatalf("expected message")
	}

	if !containsAll(got, "gog auth add", "a@b.com", "gmail") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_CredentialsMissing(t *testing.T) {
	err := &config.CredentialsMissingError{Path: "/tmp/creds.json", Cause: errNope}
	got := Format(err)

	if !containsAll(got, "gog auth credentials", "/tmp/creds.json") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_KeyNotFound(t *testing.T) {
	got := Format(keyring.ErrKeyNotFound)
	if !containsAll(got, "Secret not found", "gog auth add") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_GoogleAPIError(t *testing.T) {
	err := &ggoogleapi.Error{
		Code:    403,
		Message: "nope",
		Errors: []ggoogleapi.ErrorItem{
			{Reason: "insufficientPermissions"},
		},
	}
	got := Format(err)

	if !containsAll(got, "403", "insufficientPermissions", "nope") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_KongParseError_UnknownFlag(t *testing.T) {
	// Use real Kong parser to generate a parse error
	type TestCmd struct {
		Max int64 `name:"max" help:"Max results"`
	}

	parser, err := kong.New(&TestCmd{})
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := parser.Parse([]string{"--xyz"})
	if parseErr == nil {
		t.Fatal("expected parse error")
	}

	got := Format(parseErr)
	if !containsAll(got, "unknown flag", "--help") {
		t.Fatalf("expected help hint, got: %q", got)
	}
}

func TestFormat_KongParseError_WithSuggestion(t *testing.T) {
	// Use real Kong parser - typo should trigger suggestion
	type TestCmd struct {
		Limit int64 `name:"limit" help:"Limit results"`
	}

	parser, err := kong.New(&TestCmd{})
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := parser.Parse([]string{"--limi"})
	if parseErr == nil {
		t.Fatal("expected parse error")
	}

	got := Format(parseErr)
	// Kong provides a "did you mean" suggestion for close matches
	if strings.Contains(got, "did you mean") {
		// When Kong provides a suggestion, we should NOT add extra help
		if strings.Contains(got, "Run with --help") {
			t.Fatalf("should not add help hint when Kong provides suggestion, got: %q", got)
		}
	}
}

func TestFormat_KongParseError_UnknownFlagWithAlias(t *testing.T) {
	// Test that aliases work and don't produce errors
	type TestCmd struct {
		Max int64 `name:"max" aliases:"limit" help:"Max results"`
	}

	parser, err := kong.New(&TestCmd{})
	if err != nil {
		t.Fatal(err)
	}

	_, parseErr := parser.Parse([]string{"--limit", "10"})
	if parseErr != nil {
		t.Fatalf("--limit alias should work, got error: %v", parseErr)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}

	return true
}
