package cmd

import (
	"strings"
	"testing"

	"google.golang.org/api/people/v1"
)

func TestExportToVcf(t *testing.T) {
	tests := []struct {
		name    string
		person  *people.Person
		checkFn func(t *testing.T, vcf string)
	}{
		{
			name:   "nil person",
			person: nil,
			checkFn: func(t *testing.T, vcf string) {
				if vcf != "" {
					t.Errorf("expected empty string for nil person, got %q", vcf)
				}
			},
		},
		{
			name:   "empty person",
			person: &people.Person{},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "BEGIN:VCARD") {
					t.Error("expected VCARD header")
				}
				if !strings.Contains(vcf, "END:VCARD") {
					t.Error("expected VCARD footer")
				}
			},
		},
		{
			name: "full name from display name",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John Doe"}},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "FN:John Doe") {
					t.Errorf("expected FN:John Doe, got %q", vcf)
				}
			},
		},
		{
			name: "structured name",
			person: &people.Person{
				Names: []*people.Name{{
					GivenName:       "John",
					FamilyName:      "Doe",
					HonorificPrefix: "Mr.",
					HonorificSuffix: "III",
				}},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "FN:John Doe") {
					t.Errorf("expected FN:John Doe (DisplayName), got %q", vcf)
				}
				if !strings.Contains(vcf, "N:Doe;John") {
					t.Errorf("expected structured name Doe;John, got %q", vcf)
				}
			},
		},
		{
			name: "email addresses",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John"}},
				EmailAddresses: []*people.EmailAddress{
					{Value: "john@example.com", Type: "work"},
					{Value: "personal@example.com", Type: "home"},
				},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "EMAIL:john@example.com") {
					t.Errorf("expected work email, got %q", vcf)
				}
				if !strings.Contains(vcf, "EMAIL:personal@example.com") {
					t.Errorf("expected home email, got %q", vcf)
				}
			},
		},
		{
			name: "phone numbers",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John"}},
				PhoneNumbers: []*people.PhoneNumber{
					{Value: "+1-555-0100", Type: "work"},
					{Value: "+1-555-0101", Type: "mobile"},
				},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "TEL:+1-555-0100") {
					t.Errorf("expected work phone, got %q", vcf)
				}
				if !strings.Contains(vcf, "TEL:+1-555-0101") {
					t.Errorf("expected mobile phone, got %q", vcf)
				}
			},
		},
		{
			name: "organization with title",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John"}},
				Organizations: []*people.Organization{
					{Name: "Acme Corp", Title: "Engineer"},
				},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "ORG:Acme Corp") {
					t.Errorf("expected org, got %q", vcf)
				}
				if !strings.Contains(vcf, "TITLE:Engineer") {
					t.Errorf("expected title, got %q", vcf)
				}
			},
		},
		{
			name: "special characters escaped",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John; Doe"}},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "FN:John\\; Doe") {
					t.Errorf("expected escaped semicolon, got %q", vcf)
				}
			},
		},
		{
			name: "addresses",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John"}},
				Addresses: []*people.Address{
					{
						FormattedValue: "123 Main St, City, ST 12345",
						StreetAddress: "123 Main St",
						City:          "City",
						Region:        "ST",
						PostalCode:    "12345",
						Country:       "USA",
					},
				},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "ADR:123 Main St") {
					t.Errorf("expected address with street, got %q", vcf)
				}
			},
		},
		{
			name: "URLs",
			person: &people.Person{
				Names: []*people.Name{{DisplayName: "John"}},
				Urls: []*people.Url{
					{Value: "https://example.com"},
				},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "URL:https://example.com") {
					t.Errorf("expected URL, got %q", vcf)
				}
			},
		},
		{
			name: "biography",
			person: &people.Person{
				Names:       []*people.Name{{DisplayName: "John"}},
				Biographies: []*people.Biography{{Value: "Software developer"}},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "NOTE:Software developer") {
					t.Errorf("expected NOTE, got %q", vcf)
				}
			},
		},
		{
			name: "birthday",
			person: &people.Person{
				Names:      []*people.Name{{DisplayName: "John"}},
				Birthdays:  []*people.Birthday{{Date: &people.Date{Year: 1990, Month: 1, Day: 15}}},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "BDAY:1990-01-15") {
					t.Errorf("expected BDAY, got %q", vcf)
				}
			},
		},
		{
			name: "complete contact",
			person: &people.Person{
				Names: []*people.Name{{
					GivenName:       "John",
					FamilyName:      "Doe",
					DisplayName:     "John Doe",
					HonorificPrefix: "Mr.",
				}},
				EmailAddresses: []*people.EmailAddress{
					{Value: "john@example.com", Type: "work"},
				},
				PhoneNumbers: []*people.PhoneNumber{
					{Value: "+1-555-0100", Type: "work"},
				},
				Organizations: []*people.Organization{
					{Name: "Acme Corp", Title: "Engineer"},
				},
				Addresses: []*people.Address{
					{FormattedValue: "123 Main St, City, ST 12345"},
				},
				Urls:       []*people.Url{{Value: "https://example.com"}},
				Birthdays:  []*people.Birthday{{Date: &people.Date{Year: 1990, Month: 1, Day: 15}}},
				Biographies: []*people.Biography{{Value: "Developer"}},
			},
			checkFn: func(t *testing.T, vcf string) {
				if !strings.Contains(vcf, "BEGIN:VCARD") {
					t.Error("expected VCARD header")
				}
				if !strings.Contains(vcf, "VERSION:3.0") {
					t.Error("expected VERSION:3.0")
				}
				if !strings.Contains(vcf, "FN:John Doe") {
					t.Errorf("expected FN:John Doe, got %q", vcf)
				}
				if !strings.Contains(vcf, "N:Doe;John") {
					t.Errorf("expected N:Doe;John, got %q", vcf)
				}
				if !strings.Contains(vcf, "EMAIL:john@example.com") {
					t.Errorf("expected EMAIL, got %q", vcf)
				}
				if !strings.Contains(vcf, "TEL:+1-555-0100") {
					t.Errorf("expected TEL, got %q", vcf)
				}
				if !strings.Contains(vcf, "ORG:Acme Corp") {
					t.Errorf("expected ORG, got %q", vcf)
				}
				if !strings.Contains(vcf, "TITLE:Engineer") {
					t.Errorf("expected TITLE, got %q", vcf)
				}
				if !strings.Contains(vcf, "END:VCARD") {
					t.Error("expected VCARD footer")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vcf := exportToVcf(tt.person)
			tt.checkFn(t, vcf)
		})
	}
}

func TestVcfRelType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"home", "HOME"},
		{"work", "WORK"},
		{"mobile", "CELL"},
		{"cell", "CELL"},
		{"main", "MAIN"},
		{"other", "OTHER"},
		{"custom", "CUSTOM"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := vcfRelType(tt.input)
			if got != tt.expected {
				t.Errorf("vcfRelType(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
