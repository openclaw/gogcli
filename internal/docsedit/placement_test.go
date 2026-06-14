package docsedit

import (
	"strings"
	"testing"
)

func TestValidateAnchor(t *testing.T) {
	t.Parallel()

	one := 1
	zero := 0
	tests := []struct {
		name    string
		options AnchorOptions
		wantErr string
	}{
		{name: "unused"},
		{name: "active", options: AnchorOptions{Text: "target", Occurrence: &one, MatchCase: true}},
		{name: "empty provided", options: AnchorOptions{Provided: true}, wantErr: "empty --at"},
		{name: "occurrence without anchor", options: AnchorOptions{Occurrence: &one}, wantErr: "--occurrence requires --at"},
		{name: "match case without anchor", options: AnchorOptions{MatchCase: true}, wantErr: "--match-case requires --at"},
		{name: "invalid occurrence", options: AnchorOptions{Text: "target", Occurrence: &zero}, wantErr: "--occurrence must be > 0"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateAnchor(test.options)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateAnchor: %v", err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestPlanUpdatePlacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		options UpdatePlacementOptions
		want    Placement
		wantErr string
	}{
		{name: "end", want: Placement{Kind: PlacementEnd}},
		{
			name:    "index",
			options: UpdatePlacementOptions{Index: 7, IndexProvided: true},
			want:    Placement{Kind: PlacementIndex, Index: 7},
		},
		{
			name:    "range",
			options: UpdatePlacementOptions{ReplaceRange: " 7 : 12 "},
			want:    Placement{Kind: PlacementRange, Range: Range{Start: 7, End: 12}},
		},
		{
			name:    "anchor",
			options: UpdatePlacementOptions{Anchor: AnchorOptions{Text: "target", Provided: true}},
			want: Placement{
				Kind:   PlacementAnchor,
				Anchor: AnchorOptions{Text: "target", Provided: true},
			},
		},
		{
			name:    "invalid index",
			options: UpdatePlacementOptions{IndexProvided: true},
			wantErr: "invalid --index",
		},
		{
			name: "index and range",
			options: UpdatePlacementOptions{
				Index:         1,
				IndexProvided: true,
				ReplaceRange:  "2:3",
			},
			wantErr: "--index cannot be combined",
		},
		{
			name: "anchor and index",
			options: UpdatePlacementOptions{
				Index:         1,
				IndexProvided: true,
				Anchor:        AnchorOptions{Text: "target", Provided: true},
			},
			wantErr: "--at cannot be combined",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanUpdatePlacement(test.options)
			assertPlacementResult(t, got, err, test.want, test.wantErr)
		})
	}
}

func TestPlanInsertPlacement(t *testing.T) {
	t.Parallel()

	index := int64(7)
	zero := int64(0)
	tests := []struct {
		name    string
		options InsertPlacementOptions
		want    Placement
		wantErr string
	}{
		{name: "end", want: Placement{Kind: PlacementEnd}},
		{
			name:    "index",
			options: InsertPlacementOptions{Index: &index},
			want:    Placement{Kind: PlacementIndex, Index: 7},
		},
		{
			name:    "anchor",
			options: InsertPlacementOptions{Anchor: AnchorOptions{Text: "target", Provided: true}},
			want: Placement{
				Kind:   PlacementAnchor,
				Anchor: AnchorOptions{Text: "target", Provided: true},
			},
		},
		{name: "invalid index", options: InsertPlacementOptions{Index: &zero}, wantErr: "--index must be >= 1"},
		{
			name: "anchor and index",
			options: InsertPlacementOptions{
				Index:  &index,
				Anchor: AnchorOptions{Text: "target", Provided: true},
			},
			wantErr: "--at and --index are mutually exclusive",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanInsertPlacement(test.options)
			assertPlacementResult(t, got, err, test.want, test.wantErr)
		})
	}
}

func TestPlanEndInsertPlacement(t *testing.T) {
	t.Parallel()

	index := int64(7)
	tests := []struct {
		name    string
		options EndInsertPlacementOptions
		want    Placement
		wantErr string
	}{
		{name: "implicit end", want: Placement{Kind: PlacementEnd}},
		{name: "explicit end", options: EndInsertPlacementOptions{AtEnd: true}, want: Placement{Kind: PlacementEnd}},
		{
			name:    "index",
			options: EndInsertPlacementOptions{Index: &index},
			want:    Placement{Kind: PlacementIndex, Index: 7},
		},
		{
			name:    "anchor",
			options: EndInsertPlacementOptions{Anchor: AnchorOptions{Text: "target", Provided: true}},
			want: Placement{
				Kind:   PlacementAnchor,
				Anchor: AnchorOptions{Text: "target", Provided: true},
			},
		},
		{
			name:    "end and index",
			options: EndInsertPlacementOptions{AtEnd: true, Index: &index},
			wantErr: "--at-end and --index are mutually exclusive",
		},
		{
			name: "anchor and end",
			options: EndInsertPlacementOptions{
				AtEnd: true,
				Anchor: AnchorOptions{
					Text:     "target",
					Provided: true,
				},
			},
			wantErr: "--at cannot be combined",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanEndInsertPlacement(test.options)
			assertPlacementResult(t, got, err, test.want, test.wantErr)
		})
	}
}

func TestPlanRangePlacement(t *testing.T) {
	t.Parallel()

	start := int64(2)
	end := int64(7)
	zero := int64(0)
	tests := []struct {
		name    string
		options RangePlacementOptions
		want    Placement
		wantErr string
	}{
		{
			name:    "range",
			options: RangePlacementOptions{Start: &start, End: &end},
			want:    Placement{Kind: PlacementRange, Range: Range{Start: 2, End: 7}},
		},
		{
			name:    "anchor",
			options: RangePlacementOptions{Anchor: AnchorOptions{Text: "target", Provided: true}},
			want: Placement{
				Kind:   PlacementAnchor,
				Anchor: AnchorOptions{Text: "target", Provided: true},
			},
		},
		{name: "missing end", options: RangePlacementOptions{Start: &start}, wantErr: "provide --at or both"},
		{
			name: "anchor and range",
			options: RangePlacementOptions{
				Start:  &start,
				End:    &end,
				Anchor: AnchorOptions{Text: "target", Provided: true},
			},
			wantErr: "--at cannot be combined",
		},
		{
			name:    "invalid start",
			options: RangePlacementOptions{Start: &zero, End: &end},
			wantErr: "--start must be >= 1",
		},
		{
			name:    "invalid end",
			options: RangePlacementOptions{Start: &end, End: &start},
			wantErr: "--end must be greater",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := PlanRangePlacement(test.options)
			assertPlacementResult(t, got, err, test.want, test.wantErr)
		})
	}
}

func TestResolvePlacement(t *testing.T) {
	t.Parallel()

	anchor := TextRange{
		StartIndex: 7,
		EndIndex:   12,
		TabID:      "t.second",
		InTable:    true,
	}
	tests := []struct {
		name      string
		placement Placement
		facts     PlacementFacts
		want      ResolvedPlacement
		wantErr   string
	}{
		{
			name:      "end",
			placement: Placement{Kind: PlacementEnd},
			facts:     PlacementFacts{EndIndex: 42, TabID: "t.second"},
			want:      ResolvedPlacement{Index: 41, TabID: "t.second"},
		},
		{
			name:      "empty end",
			placement: Placement{Kind: PlacementEnd},
			want:      ResolvedPlacement{Index: 1},
		},
		{
			name:      "index",
			placement: Placement{Kind: PlacementIndex, Index: 9},
			facts:     PlacementFacts{TabID: "t.second"},
			want:      ResolvedPlacement{Index: 9, TabID: "t.second"},
		},
		{
			name: "range",
			placement: Placement{
				Kind:  PlacementRange,
				Range: Range{Start: 3, End: 8},
			},
			facts: PlacementFacts{TabID: "t.second"},
			want: ResolvedPlacement{
				Index: 3,
				Range: &Range{Start: 3, End: 8},
				TabID: "t.second",
			},
		},
		{
			name:      "anchor",
			placement: Placement{Kind: PlacementAnchor},
			facts: PlacementFacts{
				Anchor:             &anchor,
				RequiredRevisionID: "rev-1",
			},
			want: ResolvedPlacement{
				Index:              7,
				Range:              &Range{Start: 7, End: 12},
				TabID:              "t.second",
				RequiredRevisionID: "rev-1",
				Anchored:           true,
				InTable:            true,
			},
		},
		{
			name:      "missing anchor",
			placement: Placement{Kind: PlacementAnchor},
			wantErr:   "missing anchor match",
		},
		{
			name:      "unsupported",
			placement: Placement{Kind: PlacementKind(99)},
			wantErr:   "unsupported placement kind",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolvePlacement(test.placement, test.facts)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("error = %v, want %q", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("ResolvePlacement: %v", err)
			}

			if got.Index != test.want.Index ||
				got.TabID != test.want.TabID ||
				got.RequiredRevisionID != test.want.RequiredRevisionID ||
				got.Anchored != test.want.Anchored ||
				got.InTable != test.want.InTable {
				t.Fatalf("placement = %#v, want %#v", got, test.want)
			}

			switch {
			case got.Range == nil && test.want.Range == nil:
			case got.Range == nil || test.want.Range == nil:
				t.Fatalf("range = %#v, want %#v", got.Range, test.want.Range)
			case *got.Range != *test.want.Range:
				t.Fatalf("range = %#v, want %#v", got.Range, test.want.Range)
			}
		})
	}
}

func assertPlacementResult(t *testing.T, got Placement, err error, want Placement, wantErr string) {
	t.Helper()

	if wantErr != "" {
		if err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Fatalf("error = %v, want %q", err, wantErr)
		}

		return
	}

	if err != nil {
		t.Fatalf("planner error: %v", err)
	}

	if got != want {
		t.Fatalf("placement = %#v, want %#v", got, want)
	}
}
