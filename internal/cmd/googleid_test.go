package cmd

import "testing"

func TestNormalizeGoogleID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: " abc123 ", want: "abc123"},
		{in: "https://drive.google.com/file/d/FILEID/view?usp=sharing", want: "FILEID"},
		{in: "https://drive.google.com/open?id=OPENID", want: "OPENID"},
		{in: "https://drive.google.com/drive/folders/FOLDERID", want: "FOLDERID"},
		{in: "https://drive.google.com/drive/u/0/folders/FOLDERID2?foo=bar", want: "FOLDERID2"},
		{in: "https://docs.google.com/document/d/DOCID/edit", want: "DOCID"},
		{in: "https://docs.google.com/spreadsheets/d/SHEETID/edit#gid=0", want: "SHEETID"},
		{in: "https://docs.google.com/presentation/d/SLIDEID/edit", want: "SLIDEID"},
		{in: "drive.google.com/file/d/SCHEMELESS/view", want: "SCHEMELESS"},
		{in: "docs.google.com/document/d/SCHEMELESS2/edit", want: "SCHEMELESS2"},
		{in: "https://example.com/not-a-google-id", want: "https://example.com/not-a-google-id"},
	}

	for _, tt := range tests {
		if got := normalizeGoogleID(tt.in); got != tt.want {
			t.Fatalf("normalizeGoogleID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
