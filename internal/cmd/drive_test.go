package cmd

import "testing"

func TestBuildDriveListQuery(t *testing.T) {
	t.Run("adds parent and trashed", func(t *testing.T) {
		got := buildDriveListQuery("root", "")
		if got != "'root' in parents and trashed = false" {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("combines with user query", func(t *testing.T) {
		got := buildDriveListQuery("abc", "mimeType='image/png'")
		if got != "mimeType='image/png' and 'abc' in parents and trashed = false" {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("does not force trashed when user sets it", func(t *testing.T) {
		got := buildDriveListQuery("abc", "trashed = true")
		if got != "trashed = true and 'abc' in parents" {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("does not treat quoted 'trashed' as predicate", func(t *testing.T) {
		got := buildDriveListQuery("abc", "name contains 'trashed'")
		if got != "name contains 'trashed' and 'abc' in parents and trashed = false" {
			t.Fatalf("unexpected: %q", got)
		}
	})
}

func TestBuildDriveSearchQuery(t *testing.T) {
	got := buildDriveSearchQuery("hello world", false)
	if got != "fullText contains 'hello world' and trashed = false" {
		t.Fatalf("unexpected: %q", got)
	}

	t.Run("passes through filter query", func(t *testing.T) {
		got := buildDriveSearchQuery("mimeType = 'application/vnd.google-apps.document'", false)
		want := "mimeType = 'application/vnd.google-apps.document' and trashed = false"
		if got != want {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("filter query containing quoted trashed still appends trashed=false", func(t *testing.T) {
		got := buildDriveSearchQuery("name contains 'trashed'", false)
		want := "name contains 'trashed' and trashed = false"
		if got != want {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("plain text containing trashed still appends trashed=false", func(t *testing.T) {
		got := buildDriveSearchQuery("trashed", false)
		want := "fullText contains 'trashed' and trashed = false"
		if got != want {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("does not add trashed when already present", func(t *testing.T) {
		got := buildDriveSearchQuery("mimeType != 'application/vnd.google-apps.folder' and TrAsHeD = true", false)
		want := "mimeType != 'application/vnd.google-apps.folder' and TrAsHeD = true"
		if got != want {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("raw query bypasses fullText wrapping", func(t *testing.T) {
		got := buildDriveSearchQuery("hello world", true)
		want := "hello world and trashed = false"
		if got != want {
			t.Fatalf("unexpected: %q", got)
		}
	})
}

func TestEscapeDriveQueryString(t *testing.T) {
	got := escapeDriveQueryString("a'b")
	if got != "a\\'b" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormatDriveSize(t *testing.T) {
	if got := formatDriveSize(0); got != "-" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatDriveSize(1); got != "1 B" {
		t.Fatalf("unexpected: %q", got)
	}
	if got := formatDriveSize(1024); got != "1.0 KB" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestLooksLikeDriveQueryLanguage(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		// --- Should return true (filter queries) ---

		// Field comparisons
		{name: "mimeType equals", query: "mimeType = 'application/vnd.google-apps.document'", want: true},
		{name: "name not equals", query: "name != 'untitled'", want: true},
		{name: "modifiedTime greater than", query: "modifiedTime > '2024-01-01'", want: true},
		{name: "trashed equals", query: "trashed = true", want: true},
		{name: "starred equals", query: "starred = false", want: true},
		{name: "createdTime less than", query: "createdTime < '2023-06-01'", want: true},
		{name: "viewedByMeTime gte", query: "viewedByMeTime >= '2024-01-01'", want: true},
		{name: "visibility equals", query: "visibility = 'anyoneWithLink'", want: true},

		// Contains
		{name: "name contains", query: "name contains 'report'", want: true},
		{name: "fullText contains", query: "fullText contains 'budget'", want: true},

		// Membership (in)
		{name: "in parents", query: "'folder123' in parents", want: true},
		{name: "in owners", query: "'user@example.com' in owners", want: true},
		{name: "in writers", query: "'user@example.com' in writers", want: true},
		{name: "in readers", query: "'reader@example.com' in readers", want: true},

		// Has property
		{name: "properties has", query: "properties has { key='department' and value='finance' }", want: true},
		{name: "appProperties has", query: "appProperties has { key='project' and value='alpha' }", want: true},

		// sharedWithMe (case-insensitive)
		{name: "sharedWithMe exact", query: "sharedWithMe", want: true},
		{name: "sharedWithMe uppercase", query: "SHAREDWITHME", want: true},
		{name: "sharedWithMe mixed case", query: "SharedWithMe", want: true},

		// Compound queries
		{name: "compound mimeType and name contains", query: "mimeType = 'application/pdf' and name contains 'report'", want: true},
		{name: "compound trashed and starred", query: "trashed = false and starred = true", want: true},

		// --- Should return false (natural language / plain text) ---
		{name: "plain text meeting notes", query: "meeting notes", want: false},
		{name: "plain text find my documents", query: "find my documents", want: false},
		{name: "plain text trashed files", query: "trashed files", want: false},
		{name: "plain text hello world", query: "hello world", want: false},
		{name: "plain text important", query: "important", want: false},
		{name: "empty string", query: "", want: false},
		{name: "whitespace only", query: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := looksLikeDriveQueryLanguage(tt.query)
			if got != tt.want {
				t.Errorf("looksLikeDriveQueryLanguage(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}
