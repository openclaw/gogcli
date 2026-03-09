package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildColumnsRequest(t *testing.T) {
	tests := []struct {
		name         string
		cols         int
		sectionStart int64
		sectionEnd   int64
		wantLen      int
	}{
		{"no columns", 0, 1, 100, 0},
		{"negative columns", -1, 1, 100, 0},
		{"one column", 1, 1, 100, 1},
		{"two columns", 2, 1, 100, 1},
		{"three columns", 3, 10, 200, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			be := &braceExpr{Cols: tt.cols, Indent: indentNotSet}
			reqs := buildColumnsRequest(be, tt.sectionStart, tt.sectionEnd)
			assert.Len(t, reqs, tt.wantLen)

			if tt.wantLen > 0 {
				req := reqs[0]
				require.NotNil(t, req.UpdateSectionStyle)
				assert.Equal(t, tt.sectionStart, req.UpdateSectionStyle.Range.StartIndex)
				assert.Equal(t, tt.sectionEnd, req.UpdateSectionStyle.Range.EndIndex)
				assert.Len(t, req.UpdateSectionStyle.SectionStyle.ColumnProperties, tt.cols)
			}
		})
	}
}

func TestBuildColumnsRequest_NilExpr(t *testing.T) {
	reqs := buildColumnsRequest(nil, 1, 100)
	assert.Nil(t, reqs)
}

func TestBuildCheckboxRequests(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name    string
		check   *bool
		start   int64
		end     int64
		wantLen int
	}{
		{"nil check", nil, 1, 10, 0},
		{"unchecked", &falseVal, 1, 10, 1},
		{"checked", &trueVal, 1, 10, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			be := &braceExpr{Check: tt.check, Indent: indentNotSet}
			reqs := buildCheckboxRequests(be, tt.start, tt.end)
			assert.Len(t, reqs, tt.wantLen)

			if tt.wantLen > 0 {
				req := reqs[0]
				require.NotNil(t, req.CreateParagraphBullets)
				assert.Equal(t, "BULLET_CHECKBOX", req.CreateParagraphBullets.BulletPreset)
				assert.Equal(t, tt.start, req.CreateParagraphBullets.Range.StartIndex)
				assert.Equal(t, tt.end+1, req.CreateParagraphBullets.Range.EndIndex)
			}
		})
	}
}

func TestBuildCheckboxRequests_NilExpr(t *testing.T) {
	reqs := buildCheckboxRequests(nil, 1, 10)
	assert.Nil(t, reqs)
}

func TestBuildTOCRequest_Limitation(t *testing.T) {
	// TOC is not supported via API — should return nil
	be := &braceExpr{HasTOC: true, TOC: 3, Indent: indentNotSet}
	reqs := buildTOCRequest(be, 10)
	assert.Nil(t, reqs, "TOC should return nil due to API limitation")
}

func TestBuildCommentRequest_Limitation(t *testing.T) {
	// Comments are not supported via batchUpdate — should return nil
	be := &braceExpr{Comment: "needs review", Indent: indentNotSet}
	reqs := buildCommentRequest(be, 1, 10)
	assert.Nil(t, reqs, "Comment should return nil due to API limitation")
}

func TestBuildBookmarkRequest(t *testing.T) {
	tests := []struct {
		name     string
		bookmark string
		start    int64
		end      int64
		wantLen  int
	}{
		{"empty bookmark", "", 1, 10, 0},
		{"simple name", "intro", 1, 10, 1},
		{"hyphenated", "section-1", 50, 75, 1},
		{"with-underscore", "chapter_2", 100, 150, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			be := &braceExpr{Bookmark: tt.bookmark, Indent: indentNotSet}
			reqs := buildBookmarkRequest(be, tt.start, tt.end)
			assert.Len(t, reqs, tt.wantLen)

			if tt.wantLen > 0 {
				req := reqs[0]
				require.NotNil(t, req.CreateNamedRange)
				assert.Equal(t, tt.bookmark, req.CreateNamedRange.Name)
				assert.Equal(t, tt.start, req.CreateNamedRange.Range.StartIndex)
				assert.Equal(t, tt.end, req.CreateNamedRange.Range.EndIndex)
			}
		})
	}
}

func TestBuildBookmarkRequest_NilExpr(t *testing.T) {
	reqs := buildBookmarkRequest(nil, 1, 10)
	assert.Nil(t, reqs)
}

func TestParseChipURI(t *testing.T) {
	tests := []struct {
		name     string
		uri      string
		wantType ChipType
		wantVal  string
		wantOpts []string
	}{
		{"person chip", "chip://person/test@example.com", ChipTypePerson, "test@example.com", nil},
		{"date chip", "chip://date/2026-03-15", ChipTypeDate, "2026-03-15", nil},
		{"file chip", "chip://file/abc123", ChipTypeFile, "abc123", nil},
		{"place chip", "chip://place/Orlando, FL", ChipTypePlace, "Orlando, FL", nil},
		{"dropdown chip", "chip://dropdown/Draft|Review|Done", ChipTypeDropdown, "Draft|Review|Done", []string{"Draft", "Review", "Done"}},
		{"chart chip", "chip://chart/sheet123/0", ChipTypeChart, "sheet123/0", nil},
		{"bookmark chip", "chip://bookmark/section-1", ChipTypeBookmark, "section-1", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := parseChipURI(tt.uri)
			require.NotNil(t, spec)
			assert.Equal(t, tt.wantType, spec.Type)
			assert.Equal(t, tt.wantVal, spec.Value)
			if tt.wantOpts != nil {
				assert.Equal(t, tt.wantOpts, spec.Options)
			}
		})
	}
}

func TestParseChipURI_Invalid(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"not chip scheme", "https://example.com"},
		{"no path", "chip://"},
		{"only type", "chip://person"},
		{"unknown type", "chip://unknown/value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := parseChipURI(tt.uri)
			assert.Nil(t, spec)
		})
	}
}

func TestBuildPersonChipRequest(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		index   int64
		wantLen int
	}{
		{"empty email", "", 10, 0},
		{"valid email", "test@example.com", 10, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqs := buildPersonChipRequest(tt.email, tt.index)
			assert.Len(t, reqs, tt.wantLen)

			if tt.wantLen > 0 {
				req := reqs[0]
				require.NotNil(t, req.InsertPerson)
				assert.Equal(t, tt.email, req.InsertPerson.PersonProperties.Email)
				assert.Equal(t, tt.index, req.InsertPerson.Location.Index)
			}
		})
	}
}

func TestBuildChipRequests_Person(t *testing.T) {
	be := &braceExpr{URL: "chip://person/test@example.com", Indent: indentNotSet}
	reqs := buildChipRequests(be, 10)
	require.Len(t, reqs, 1)
	assert.NotNil(t, reqs[0].InsertPerson)
	assert.Equal(t, "test@example.com", reqs[0].InsertPerson.PersonProperties.Email)
}

func TestBuildChipRequests_UnsupportedTypes(t *testing.T) {
	// These chip types are not supported via batchUpdate API
	unsupported := []string{
		"chip://date/2026-03-15",
		"chip://file/doc123",
		"chip://place/Orlando",
		"chip://dropdown/A|B|C",
		"chip://chart/sheet/0",
	}

	for _, uri := range unsupported {
		t.Run(uri, func(t *testing.T) {
			be := &braceExpr{URL: uri, Indent: indentNotSet}
			reqs := buildChipRequests(be, 10)
			assert.Nil(t, reqs, "unsupported chip type should return nil")
		})
	}
}

func TestBuildChipRequests_NonChipURL(t *testing.T) {
	be := &braceExpr{URL: "https://example.com", Indent: indentNotSet}
	reqs := buildChipRequests(be, 10)
	assert.Nil(t, reqs, "non-chip URL should return nil")
}

func TestHasBraceStructuralFeatures(t *testing.T) {
	trueVal := true

	tests := []struct {
		name string
		be   *braceExpr
		want bool
	}{
		{"nil", nil, false},
		{"empty", &braceExpr{Indent: indentNotSet}, false},
		{"cols", &braceExpr{Cols: 2, Indent: indentNotSet}, true},
		{"check", &braceExpr{Check: &trueVal, Indent: indentNotSet}, true},
		{"toc", &braceExpr{HasTOC: true, Indent: indentNotSet}, true},
		{"comment", &braceExpr{Comment: "test", Indent: indentNotSet}, true},
		{"bookmark", &braceExpr{Bookmark: "name", Indent: indentNotSet}, true},
		{"chip url", &braceExpr{URL: "chip://person/a@b.com", Indent: indentNotSet}, true},
		{"regular url", &braceExpr{URL: "https://example.com", Indent: indentNotSet}, false},
		{"bold only", &braceExpr{Bold: &trueVal, Indent: indentNotSet}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasBraceStructuralFeatures(tt.be)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name      string
		dateStr   string
		wantYear  int
		wantMonth int
		wantDay   int
		wantOK    bool
	}{
		{"ISO date", "2026-03-15", 2026, 3, 15, true},
		{"slash date", "2026/03/15", 2026, 3, 15, true},
		{"US date dash", "03-15-2026", 2026, 3, 15, true},
		{"US date slash", "03/15/2026", 2026, 3, 15, true},
		{"long format", "March 15, 2026", 2026, 3, 15, true},
		{"short format", "Mar 15, 2026", 2026, 3, 15, true},
		{"invalid", "not-a-date", 0, 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, day, ok := parseDate(tt.dateStr)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantYear, year)
				assert.Equal(t, tt.wantMonth, month)
				assert.Equal(t, tt.wantDay, day)
			}
		})
	}
}

func TestBuildDateFallbackText(t *testing.T) {
	tests := []struct {
		name    string
		dateStr string
		want    string
	}{
		{"ISO date", "2026-03-15", "2026-03-15"},
		{"long format", "March 15, 2026", "2026-03-15"},
		{"invalid", "not-a-date", "not-a-date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDateFallbackText(tt.dateStr)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveChipURL(t *testing.T) {
	tests := []struct {
		name         string
		chipURL      string
		wantURL      string
		wantFallback string
	}{
		{"regular url", "https://example.com", "https://example.com", ""},
		{"person chip", "chip://person/test@example.com", "", ""},
		{"bookmark chip", "chip://bookmark/section-1", "#section-1", ""},
		{"date chip", "chip://date/2026-03-15", "", "2026-03-15"},
		{"file chip", "chip://file/doc123", "https://docs.google.com/document/d/doc123", ""},
		{"place chip", "chip://place/Orlando, FL", "https://maps.google.com/?q=Orlando%2C+FL", "Orlando, FL"},
		{"dropdown chip", "chip://dropdown/A|B|C", "", "A / B / C"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, fallback := resolveChipURL(tt.chipURL)
			assert.Equal(t, tt.wantURL, url)
			assert.Equal(t, tt.wantFallback, fallback)
		})
	}
}

func TestGetCheckboxState(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name string
		be   *braceExpr
		want string
	}{
		{"nil expr", nil, ""},
		{"nil check", &braceExpr{Indent: indentNotSet}, ""},
		{"checked", &braceExpr{Check: &trueVal, Indent: indentNotSet}, "checked"},
		{"unchecked", &braceExpr{Check: &falseVal, Indent: indentNotSet}, "unchecked"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getCheckboxState(tt.be)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStripChipPrefix(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{"person chip", "chip://person/test@example.com", "test@example.com"},
		{"date chip", "chip://date/2026-03-15", "2026-03-15"},
		{"regular url", "https://example.com", "https://example.com"},
		{"no value", "chip://person", "person"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripChipPrefix(tt.uri)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsPersonChip(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{"person chip", "chip://person/test@example.com", true},
		{"date chip", "chip://date/2026-03-15", false},
		{"regular url", "https://example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPersonChip(tt.uri)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractPersonEmail(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want string
	}{
		{"valid person chip", "chip://person/test@example.com", "test@example.com"},
		{"not person chip", "chip://date/2026-03-15", ""},
		{"regular url", "https://example.com", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPersonEmail(tt.uri)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseChartChip(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		wantSheet string
		wantIndex int
		wantOK    bool
	}{
		{"valid chart", "chip://chart/sheet123/0", "sheet123", 0, true},
		{"chart index 2", "chip://chart/abc/2", "abc", 2, true},
		{"not chart", "chip://person/a@b.com", "", 0, false},
		{"missing index", "chip://chart/sheet", "", 0, false},
		{"invalid index", "chip://chart/sheet/notanumber", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sheet, idx, ok := parseChartChip(tt.uri)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantSheet, sheet)
				assert.Equal(t, tt.wantIndex, idx)
			}
		})
	}
}

func TestBuildStructuralRequests(t *testing.T) {
	trueVal := true

	t.Run("columns only", func(t *testing.T) {
		be := &braceExpr{Cols: 2, Indent: indentNotSet}
		colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(be, 10, 20, 1, 100)
		assert.Len(t, colReqs, 1)
		assert.Nil(t, bulletReqs)
		assert.Nil(t, anchorReqs)
		assert.Nil(t, chipReqs)
	})

	t.Run("checkbox only", func(t *testing.T) {
		be := &braceExpr{Check: &trueVal, Indent: indentNotSet}
		colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(be, 10, 20, 1, 100)
		assert.Nil(t, colReqs)
		assert.Len(t, bulletReqs, 1)
		assert.Nil(t, anchorReqs)
		assert.Nil(t, chipReqs)
	})

	t.Run("bookmark only", func(t *testing.T) {
		be := &braceExpr{Bookmark: "intro", Indent: indentNotSet}
		colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(be, 10, 20, 1, 100)
		assert.Nil(t, colReqs)
		assert.Nil(t, bulletReqs)
		assert.Len(t, anchorReqs, 1)
		assert.Nil(t, chipReqs)
	})

	t.Run("person chip only", func(t *testing.T) {
		be := &braceExpr{URL: "chip://person/a@b.com", Indent: indentNotSet}
		colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(be, 10, 20, 1, 100)
		assert.Nil(t, colReqs)
		assert.Nil(t, bulletReqs)
		assert.Nil(t, anchorReqs)
		assert.Len(t, chipReqs, 1)
	})

	t.Run("multiple features", func(t *testing.T) {
		be := &braceExpr{
			Cols:     2,
			Check:    &trueVal,
			Bookmark: "test",
			Indent:   -1,
		}
		colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(be, 10, 20, 1, 100)
		assert.Len(t, colReqs, 1)
		assert.Len(t, bulletReqs, 1)
		assert.Len(t, anchorReqs, 1)
		assert.Nil(t, chipReqs)
	})

	t.Run("nil expr", func(t *testing.T) {
		colReqs, bulletReqs, anchorReqs, chipReqs := buildStructuralRequests(nil, 10, 20, 1, 100)
		assert.Nil(t, colReqs)
		assert.Nil(t, bulletReqs)
		assert.Nil(t, anchorReqs)
		assert.Nil(t, chipReqs)
	})
}
