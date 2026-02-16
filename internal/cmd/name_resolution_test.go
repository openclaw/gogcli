package cmd

import (
	"context"
	"testing"
)

// --- resolveTasklistID bypass/short-circuit cases ---

func TestResolveTasklistID_EmptyString(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestResolveTasklistID_WhitespaceOnly(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "   ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestResolveTasklistID_DefaultLowercase(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "@default" {
		t.Fatalf("expected %q, got %q", "@default", got)
	}
}

func TestResolveTasklistID_DefaultMixedCase(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "Default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "@default" {
		t.Fatalf("expected %q, got %q", "@default", got)
	}
}

func TestResolveTasklistID_DefaultUppercase(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "DEFAULT")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "@default" {
		t.Fatalf("expected %q, got %q", "@default", got)
	}
}

func TestResolveTasklistID_AtDefault(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "@default")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "@default" {
		t.Fatalf("expected %q, got %q", "@default", got)
	}
}

func TestResolveTasklistID_DefaultWithLeadingWhitespace(t *testing.T) {
	got, err := resolveTasklistID(context.TODO(), nil, "  default  ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "@default" {
		t.Fatalf("expected %q, got %q", "@default", got)
	}
}

func TestResolveTasklistID_LongOpaqueID(t *testing.T) {
	id := "MDQ2NTI3MjEwMzA0NjUyOTM1NzA6MDow"
	got, err := resolveTasklistID(context.TODO(), nil, id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != id {
		t.Fatalf("expected %q, got %q", id, got)
	}
}

func TestResolveTasklistID_Exactly16Chars(t *testing.T) {
	id := "abcdefghij012345"
	got, err := resolveTasklistID(context.TODO(), nil, id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != id {
		t.Fatalf("expected %q, got %q", id, got)
	}
}

func TestResolveTasklistID_LongStringWithSpacesNotBypassed(t *testing.T) {
	// A long string that contains spaces should NOT be treated as an ID.
	// This would require an API call, so passing nil svc should panic.
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic due to nil service for API resolution path")
		}
	}()
	_, _ = resolveTasklistID(context.TODO(), nil, "my very long tasklist name here")
}

// --- resolveCalendarID bypass/short-circuit cases ---

func TestResolveCalendarID_EmptyString(t *testing.T) {
	got, err := resolveCalendarID(context.TODO(), nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestResolveCalendarID_WhitespaceOnly(t *testing.T) {
	got, err := resolveCalendarID(context.TODO(), nil, "   ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestResolveCalendarID_PrimaryLowercase(t *testing.T) {
	got, err := resolveCalendarID(context.TODO(), nil, "primary")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "primary" {
		t.Fatalf("expected %q, got %q", "primary", got)
	}
}

func TestResolveCalendarID_PrimaryMixedCase(t *testing.T) {
	got, err := resolveCalendarID(context.TODO(), nil, "Primary")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "primary" {
		t.Fatalf("expected %q, got %q", "primary", got)
	}
}

func TestResolveCalendarID_PrimaryUppercase(t *testing.T) {
	got, err := resolveCalendarID(context.TODO(), nil, "PRIMARY")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "primary" {
		t.Fatalf("expected %q, got %q", "primary", got)
	}
}

func TestResolveCalendarID_PrimaryWithWhitespace(t *testing.T) {
	got, err := resolveCalendarID(context.TODO(), nil, "  primary  ")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "primary" {
		t.Fatalf("expected %q, got %q", "primary", got)
	}
}

func TestResolveCalendarID_EmailAddress(t *testing.T) {
	id := "user@example.com"
	got, err := resolveCalendarID(context.TODO(), nil, id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != id {
		t.Fatalf("expected %q, got %q", id, got)
	}
}

func TestResolveCalendarID_GroupCalendarID(t *testing.T) {
	id := "company.com_abcdef1234@group.calendar.google.com"
	got, err := resolveCalendarID(context.TODO(), nil, id)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != id {
		t.Fatalf("expected %q, got %q", id, got)
	}
}

func TestResolveCalendarID_NameWithoutAtTriggeresAPI(t *testing.T) {
	// A plain name without "@" would trigger API resolution.
	// Passing nil svc should panic.
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic due to nil service for API resolution path")
		}
	}()
	_, _ = resolveCalendarID(context.TODO(), nil, "Work Calendar")
}
