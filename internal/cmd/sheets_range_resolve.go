package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/selectorutil"
)

type spreadsheetRangeCatalog struct {
	SheetIDsByTitle map[string]int64
	SheetTitlesByID map[int64]string
	NamedRanges     []*sheets.NamedRange
	Sheets          []*sheets.SheetProperties
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
	props := make([]*sheets.SheetProperties, 0, len(resp.Sheets))
	for _, sh := range resp.Sheets {
		if sh == nil || sh.Properties == nil {
			continue
		}
		props = append(props, sh.Properties)
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
		Sheets:          props,
	}, nil
}

// resolveGridRangeWithCatalog accepts either:
// - A1 notation with sheet name (e.g. Sheet1!A1:B2), or
// - a named range name (e.g. MyNamedRange)
func resolveGridRangeWithCatalog(input string, catalog *spreadsheetRangeCatalog, label string) (*sheets.GridRange, error) {
	in := cleanRange(strings.TrimSpace(input))
	if in == "" {
		return nil, usagef("empty %s range", label)
	}
	if catalog == nil {
		return nil, fmt.Errorf("missing spreadsheet range catalog")
	}

	// If the user provided an A1 reference with a sheet name, keep existing
	// behavior (and error messages) for A1 parsing.
	if strings.Contains(in, "!") {
		r, err := parseSheetRange(in, label)
		if err != nil {
			return nil, err
		}
		grid, err := gridRangeFromMap(r, catalog.SheetIDsByTitle, label)
		if err != nil {
			return nil, err
		}
		return grid, nil
	}

	// Try resolving as a named range name (case-insensitive exact match).
	nr, found, err := resolveNamedRangeByNameOrID(in, catalog.NamedRanges)
	if err != nil {
		return nil, err
	}
	if found && nr != nil && nr.Range != nil {
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
			fs = append(fs, "SheetId")
			gr.ForceSendFields = fs
		}
		return &gr, nil
	}

	// If it looks like A1 but doesn't include a sheet name, preserve the prior
	// strict requirement for A1-with-sheet ranges for GridRange-based operations.
	if _, a1Err := parseA1Range(in); a1Err == nil {
		return nil, usagef("%s range must include a sheet name (e.g. Sheet1!A1:B2) or be a named range", label)
	}

	return nil, usagef("unknown named range %q", in)
}

// resolveNamedRangeByNameOrID finds a named range by:
// - exact ID match, or
// - case-insensitive exact name match (errors if ambiguous).
//
// It returns found=false when no matches exist.
func resolveNamedRangeByNameOrID(input string, namedRanges []*sheets.NamedRange) (*sheets.NamedRange, bool, error) {
	in := strings.TrimSpace(input)
	if in == "" {
		return nil, false, nil
	}

	options := make([]selectorutil.Match, 0, len(namedRanges))
	for _, nr := range namedRanges {
		if nr == nil {
			continue
		}
		options = append(options, selectorutil.Match{
			ID:   strings.TrimSpace(nr.NamedRangeId),
			Name: strings.TrimSpace(nr.Name),
		})
	}

	match, found, ambiguous := selectorutil.FindByIDOrCaseFoldName(in, options)
	if !found {
		if len(ambiguous) > 0 {
			parts := make([]string, 0, len(ambiguous))
			for _, match := range ambiguous {
				label := match.Name
				if label == "" {
					label = "(unnamed)"
				}
				parts = append(parts, fmt.Sprintf("%s (%s)", label, match.ID))
			}
			return nil, false, usagef("ambiguous named range %q; matches: %s", in, strings.Join(parts, ", "))
		}
		return nil, false, nil
	}
	for _, nr := range namedRanges {
		if nr != nil && strings.TrimSpace(nr.NamedRangeId) == match.ID {
			return nr, true, nil
		}
	}
	return nil, false, fmt.Errorf("named range match disappeared (id=%q)", match.ID)
}
