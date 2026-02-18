package cmd

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/cloudidentity/v1"
)

func TestWrapCloudIdentityError(t *testing.T) {
	err := wrapCloudIdentityError(errors.New("accessNotConfigured: boom"), "user@company.com")
	if !strings.Contains(err.Error(), "Cloud Identity API is not enabled") {
		t.Fatalf("unexpected error: %v", err)
	}

	err = wrapCloudIdentityError(errors.New("insufficientPermissions: nope"), "user@company.com")
	if !strings.Contains(err.Error(), "Insufficient permissions") {
		t.Fatalf("unexpected error: %v", err)
	}

	other := errors.New("other")
	if !errors.Is(wrapCloudIdentityError(other, "user@company.com"), other) {
		t.Fatalf("expected passthrough error")
	}
}

func TestWrapCloudIdentityError_ConsumerAccount(t *testing.T) {
	err := wrapCloudIdentityError(errors.New("badRequest: Request contains an invalid argument."), "person@gmail.com")
	if !strings.Contains(err.Error(), "consumer accounts") {
		t.Fatalf("unexpected consumer error: %v", err)
	}
}

func TestGetRelationType(t *testing.T) {
	if got := getRelationType("DIRECT"); got != "direct" {
		t.Fatalf("unexpected relation: %q", got)
	}
	if got := getRelationType("INDIRECT"); got != "indirect" {
		t.Fatalf("unexpected relation: %q", got)
	}
	if got := getRelationType("CUSTOM"); got != "CUSTOM" {
		t.Fatalf("unexpected relation: %q", got)
	}
}

func TestGetMemberRole(t *testing.T) {
	if got := getMemberRole(nil); got != "MEMBER" {
		t.Fatalf("unexpected role: %q", got)
	}
	got := getMemberRole([]*cloudidentity.MembershipRole{
		{Name: "MEMBER"},
		{Name: "OWNER"},
	})
	if got != "OWNER" {
		t.Fatalf("unexpected role: %q", got)
	}
	got = getMemberRole([]*cloudidentity.MembershipRole{
		{Name: "MANAGER"},
	})
	if got != "MANAGER" {
		t.Fatalf("unexpected role: %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("unexpected truncate: %q", got)
	}
	if got := truncate("hello world", 5); got != "he..." {
		t.Fatalf("unexpected truncate: %q", got)
	}
	if got := truncate("hello", 3); got != "hel" {
		t.Fatalf("unexpected truncate: %q", got)
	}
}

func TestSearchTransitiveGroupsQuery(t *testing.T) {
	got := searchTransitiveGroupsQuery("person@example.com")
	if !strings.Contains(got, "member_key_id == 'person@example.com'") {
		t.Fatalf("missing member_key_id clause: %q", got)
	}
	if !strings.Contains(got, "'"+groupLabelDiscussionForum+"' in labels") {
		t.Fatalf("missing discussion label clause: %q", got)
	}
	if !strings.Contains(got, "'"+groupLabelDynamic+"' in labels") {
		t.Fatalf("missing dynamic label clause: %q", got)
	}
}

func TestSearchTransitiveGroupsQuery_EscapesSingleQuote(t *testing.T) {
	got := searchTransitiveGroupsQuery("o'connor@example.com")
	if !strings.Contains(got, "member_key_id == 'o\\'connor@example.com'") {
		t.Fatalf("expected escaped single quote: %q", got)
	}
}
