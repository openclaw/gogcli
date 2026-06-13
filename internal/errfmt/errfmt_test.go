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

func TestFormat_AuthRequired_ServiceAccountOnly(t *testing.T) {
	for _, service := range []string{"admin", "admin directory", "admin orgunits", "groups", "keep"} {
		t.Run(service, func(t *testing.T) {
			err := &gogapi.AuthRequiredError{Service: service, Email: "a@b.com", Cause: keyring.ErrKeyNotFound}
			got := Format(err)

			if !containsAll(got, "No auth for "+service+" a@b.com", "gog auth service-account set a@b.com") {
				t.Fatalf("unexpected: %q", got)
			}

			if strings.Contains(got, "--services "+service) {
				t.Fatalf("must not suggest unsupported admin service flag: %q", got)
			}
		})
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

func TestFormat_UserFacingError(t *testing.T) {
	err := NewUserFacingError("friendly", errNope)
	got := Format(err)

	if got != "friendly" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_UserFacingErrorWrapsSentinel(t *testing.T) {
	err := NewUserFacingError("friendly", keyring.ErrKeyNotFound)
	got := Format(err)

	if got != "friendly" {
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

func TestFormat_GoogleAPIError_AccessNotConfiguredHint(t *testing.T) {
	err := &ggoogleapi.Error{
		Code: 403,
		Message: "Google Drive API has not been used in project 123 before or it is disabled. " +
			"Enable it by visiting https://console.developers.google.com/apis/api/drive.googleapis.com/overview?project=123",
		Errors: []ggoogleapi.ErrorItem{
			{Reason: "accessNotConfigured"},
		},
	}
	got := Format(err)

	if !containsAll(got, "Drive API is not enabled", "drive.googleapis.com", "--services drive") {
		t.Fatalf("unexpected: %q", got)
	}

	if !strings.Contains(got, "overview?project=123") {
		t.Fatalf("expected project-scoped enable URL, got: %q", got)
	}

	if strings.Contains(got, "Google API error") {
		t.Fatalf("expected user-facing enablement hint, got: %q", got)
	}
}

func TestFormat_GoogleAPIError_ServiceAccountOnlyDisabledAPI(t *testing.T) {
	for _, tc := range []struct {
		name    string
		apiName string
		service string
	}{
		{name: "admin", apiName: "admin.googleapis.com", service: "admin"},
		{name: "keep", apiName: "keep.googleapis.com", service: "keep"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := &ggoogleapi.Error{
				Code: 403,
				Message: "API has not been used. Enable it by visiting " +
					"https://console.developers.google.com/apis/api/" + tc.apiName + "/overview?project=123",
				Errors: []ggoogleapi.ErrorItem{
					{Reason: "accessNotConfigured"},
				},
			}

			got := Format(err)
			if !containsAll(got, "service-account set", "overview?project=123") {
				t.Fatalf("unexpected: %q", got)
			}

			if strings.Contains(got, "--services "+tc.service) || strings.Contains(got, "gog auth add") {
				t.Fatalf("must not suggest unsupported OAuth flow: %q", got)
			}
		})
	}
}

func TestFormat_GoogleAPIError_DriveActivityHint(t *testing.T) {
	err := &ggoogleapi.Error{
		Code: 403,
		Message: "Google Drive Activity API has not been used in project 123 before or it is disabled. " +
			"Enable it by visiting https://console.developers.google.com/apis/api/driveactivity.googleapis.com/overview?project=123",
		Errors: []ggoogleapi.ErrorItem{
			{Reason: "accessNotConfigured"},
		},
	}
	got := Format(err)

	if !containsAll(got, "Drive Activity API is not enabled", "driveactivity.googleapis.com", "--services driveactivity") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormat_GoogleAPIError_AnalyticsAdminHint(t *testing.T) {
	err := &ggoogleapi.Error{
		Code: 403,
		Message: "Google Analytics Admin API has not been used in project 123 before or it is disabled. " +
			"Enable it by visiting https://console.developers.google.com/apis/api/analyticsadmin.googleapis.com/overview?project=123",
		Errors: []ggoogleapi.ErrorItem{
			{Reason: "accessNotConfigured"},
		},
	}
	got := Format(err)

	if !containsAll(got, "Analytics Admin API is not enabled", "analyticsadmin.googleapis.com", "--services analytics") {
		t.Fatalf("unexpected: %q", got)
	}

	if strings.Contains(got, "Admin SDK API") {
		t.Fatalf("expected Analytics Admin hint, got: %q", got)
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
