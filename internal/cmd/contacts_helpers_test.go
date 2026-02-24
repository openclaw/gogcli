package cmd

import (
	"testing"

	"google.golang.org/api/people/v1"
)

func TestPrimaryName_EdgeCases(t *testing.T) {
	if got := primaryName(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryName(&people.Person{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryName(&people.Person{Names: []*people.Name{nil}}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	p1 := &people.Person{Names: []*people.Name{{DisplayName: "Ada Lovelace"}}}
	if got := primaryName(p1); got != "Ada Lovelace" {
		t.Fatalf("unexpected: %q", got)
	}

	p2 := &people.Person{Names: []*people.Name{{GivenName: "Ada", FamilyName: "Lovelace"}}}
	if got := primaryName(p2); got != "Ada Lovelace" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestPrimaryEmailAndPhone_EdgeCases(t *testing.T) {
	if got := primaryEmail(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryEmail(&people.Person{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryEmail(&people.Person{EmailAddresses: []*people.EmailAddress{nil}}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryEmail(&people.Person{EmailAddresses: []*people.EmailAddress{{Value: "a@b.com"}}}); got != "a@b.com" {
		t.Fatalf("unexpected: %q", got)
	}

	if got := primaryPhone(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryPhone(&people.Person{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryPhone(&people.Person{PhoneNumbers: []*people.PhoneNumber{nil}}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryPhone(&people.Person{PhoneNumbers: []*people.PhoneNumber{{Value: "+1"}}}); got != "+1" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestPrimaryBirthday_EdgeCases(t *testing.T) {
	if got := primaryBirthday(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryBirthday(&people.Person{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := primaryBirthday(&people.Person{Birthdays: []*people.Birthday{nil}}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}

	p1 := &people.Person{Birthdays: []*people.Birthday{{Date: &people.Date{Year: 1815, Month: 12, Day: 10}}}}
	if got := primaryBirthday(p1); got != "1815-12-10" {
		t.Fatalf("unexpected: %q", got)
	}

	p2 := &people.Person{Birthdays: []*people.Birthday{{Date: &people.Date{Month: 12, Day: 10}}}}
	if got := primaryBirthday(p2); got != "12-10" {
		t.Fatalf("unexpected: %q", got)
	}

	p3 := &people.Person{Birthdays: []*people.Birthday{{Date: &people.Date{Year: 1815}}}}
	if got := primaryBirthday(p3); got != "1815" {
		t.Fatalf("unexpected: %q", got)
	}

	p4 := &people.Person{Birthdays: []*people.Birthday{{Text: "Dec 10"}}}
	if got := primaryBirthday(p4); got != "Dec 10" {
		t.Fatalf("unexpected: %q", got)
	}

	p5 := &people.Person{Birthdays: []*people.Birthday{
		{Date: &people.Date{Year: 1900, Month: 1, Day: 1}},
		{Date: &people.Date{Year: 1815, Month: 12, Day: 10}, Metadata: &people.FieldMetadata{Primary: true}},
	}}
	if got := primaryBirthday(p5); got != "1815-12-10" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestParseCustomUserDefined_InvalidInput(t *testing.T) {
	if _, _, err := parseCustomUserDefined([]string{"bad"}, true); err == nil {
		t.Fatalf("expected error for missing '='")
	}
	if _, _, err := parseCustomUserDefined([]string{"=value"}, true); err == nil {
		t.Fatalf("expected error for empty key")
	}
	if _, _, err := parseCustomUserDefined([]string{""}, false); err == nil {
		t.Fatalf("expected error for empty custom value")
	}
}

func TestParseCustomUserDefined_ValidInput(t *testing.T) {
	fields, clear, err := parseCustomUserDefined([]string{"team=devops", " repo = gog"}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clear {
		t.Fatalf("did not expect clear")
	}
	if len(fields) != 2 || fields[0].Key != "team" || fields[0].Value != "devops" || fields[1].Key != "repo" || fields[1].Value != "gog" {
		t.Fatalf("unexpected fields: %#v", fields)
	}
}

func TestParseCustomUserDefined_ClearAll(t *testing.T) {
	fields, clear, err := parseCustomUserDefined([]string{""}, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clear {
		t.Fatalf("expected clear")
	}
	if len(fields) != 0 {
		t.Fatalf("expected empty fields, got %v", len(fields))
	}
}
