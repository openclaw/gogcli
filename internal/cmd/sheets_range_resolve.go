package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/sheets/v4"
)

type spreadsheetRangeCatalog struct {
	SheetIDsByTitle map[string]int64
	SheetTitlesByID map[int64]string
	NamedRanges     []*sheets.NamedRange
}

func fetchSpreadsheetRangeCatalog(ctx context.Context, svc *sheets.Service, spreadsheetID string) (*spreadsheetRangeCatalog, error) {
	call := svc.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title)),namedRanges(namedRangeId,name,range)")
	if ctx != nil {
		call = call.Context(ctx)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("get spreadsheet metadata: %w", err)
	}

	idsByTitle := make(map[string]int64, len(resp.Sheets))
	titlesByID := make(map[int64]string, len(resp.Sheets))
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		// Keep exact title bytes for map key parity with parsed quoted A1 names.
		title := sh.Properties.Title
		if title == "" {
			continue
		}
		idsByTitle[title] = sh.Properties.SheetId
		titlesByID[sh.Properties.SheetId] = title
	}

	return &spreadsheetRangeCatalog{
		SheetIDsByTitle: idsByTitle,
		SheetTitlesByID: titlesByID,
		NamedRanges:     resp.NamedRanges,
	}, nil
}

// resolveGridRangeWithCatalog accepts either:
// - A1 notation with sheet name (e.g. Sheet1!A1:B2), or
// - a named range name (e.g. MyNamedRange)
//
// It returns a GridRange and a stable display name (A1 or the named range name).
func resolveGridRangeWithCatalog(input string, catalog *spreadsheetRangeCatalog, label string) (*sheets.GridRange, string, error) {
	in := cleanRange(strings.TrimSpace(input))
	if in == "" {
		return nil, "", usagef("empty %s range", label)
	}
	if catalog == nil {
		return nil, "", fmt.Errorf("missing spreadsheet range catalog")
	}

	// If the user provided an A1 reference with a sheet name, keep existing
	// behavior (and error messages) for A1 parsing.
	if strings.Contains(in, "!") {
		r, err := parseSheetRange(in, label)
		if err != nil {
			return nil, "", err
		}
		grid, err := gridRangeFromMap(r, catalog.SheetIDsByTitle, label)
		if err != nil {
			return nil, "", err
		}
		return grid, in, nil
	}

	// Try resolving as a named range name (case-insensitive exact match).
	nr, err := resolveNamedRangeByNameOrID(in, catalog.NamedRanges)
	if err != nil {
		return nil, "", err
	}
	if nr != nil && nr.Range != nil {
		// Make sure sheetId is always sent even when it's 0.
		gr := *nr.Range
		needSheetID := true
		for _, f := range gr.ForceSendFields {
			if f == "SheetId" {
				needSheetID = false
				break
			}
		}
		if needSheetID {
			fs := make([]string, len(gr.ForceSendFields), len(gr.ForceSendFields)+1)
			copy(fs, gr.ForceSendFields)
			gr.ForceSendFields = append(fs, "SheetId")
		}
		return &gr, nr.Name, nil
	}

	// If it looks like A1 but doesn't include a sheet name, preserve the prior
	// strict requirement for A1-with-sheet ranges for GridRange-based operations.
	if _, a1Err := parseA1Range(in); a1Err == nil {
		return nil, "", usagef("%s range must include a sheet name (e.g. Sheet1!A1:B2) or be a named range", label)
	}

	return nil, "", usagef("unknown named range %q", in)
}

type namedRangeMatch struct {
	ID   string
	Name string
}

// resolveNamedRangeByNameOrID finds a named range by:
// - exact ID match, or
// - case-insensitive exact name match (errors if ambiguous).
//
// It returns nil,nil when no matches exist.
func resolveNamedRangeByNameOrID(input string, namedRanges []*sheets.NamedRange) (*sheets.NamedRange, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil, nil
	}

	// Prefer an exact ID match without any ambiguity.
	for _, nr := range namedRanges {
		if nr == nil {
			continue
		}
		if strings.TrimSpace(nr.NamedRangeId) == in {
			return nr, nil
		}
	}

	var matches []namedRangeMatch
	for _, nr := range namedRanges {
		if nr == nil {
			continue
		}
		name := strings.TrimSpace(nr.Name)
		if name == "" {
			continue
		}
		if strings.EqualFold(name, in) {
			matches = append(matches, namedRangeMatch{ID: strings.TrimSpace(nr.NamedRangeId), Name: name})
		}
	}

	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) == 1 {
		for _, nr := range namedRanges {
			if nr != nil && strings.TrimSpace(nr.NamedRangeId) == matches[0].ID {
				return nr, nil
			}
		}
		// Shouldn't happen, but be safe.
		return nil, fmt.Errorf("named range match disappeared (id=%q)", matches[0].ID)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Name == matches[j].Name {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].Name < matches[j].Name
	})
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		label := m.Name
		if label == "" {
			label = "(unnamed)"
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", label, m.ID))
	}
	return nil, usagef("ambiguous named range %q; matches: %s", in, strings.Join(parts, ", "))
}
