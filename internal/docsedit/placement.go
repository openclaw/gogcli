package docsedit

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errMissingAnchorMatch       = errors.New("missing anchor match")
	errUnsupportedPlacementKind = errors.New("unsupported placement kind")
)

type PlacementKind uint8

const (
	PlacementEnd PlacementKind = iota
	PlacementIndex
	PlacementRange
	PlacementAnchor
)

type AnchorOptions struct {
	Text       string
	Provided   bool
	Occurrence *int
	MatchCase  bool
}

type Placement struct {
	Kind   PlacementKind
	Index  int64
	Range  Range
	Anchor AnchorOptions
}

type UpdatePlacementOptions struct {
	Index         int64
	IndexProvided bool
	ReplaceRange  string
	Anchor        AnchorOptions
}

type InsertPlacementOptions struct {
	Index  *int64
	Anchor AnchorOptions
}

type EndInsertPlacementOptions struct {
	Index  *int64
	AtEnd  bool
	Anchor AnchorOptions
}

type RangePlacementOptions struct {
	Start  *int64
	End    *int64
	Anchor AnchorOptions
}

type PlacementFacts struct {
	EndIndex           int64
	TabID              string
	Anchor             *TextRange
	RequiredRevisionID string
}

type ResolvedPlacement struct {
	Index              int64
	Range              *Range
	TabID              string
	RequiredRevisionID string
	Anchored           bool
	InTable            bool
}

func ValidateAnchor(options AnchorOptions) error {
	if options.Text == "" {
		switch {
		case options.Provided:
			return invalid("empty --at")
		case options.Occurrence != nil:
			return invalid("--occurrence requires --at")
		case options.MatchCase:
			return invalid("--match-case requires --at")
		default:
			return nil
		}
	}

	if options.Occurrence != nil && *options.Occurrence <= 0 {
		return invalid("--occurrence must be > 0")
	}

	return nil
}

func PlanUpdatePlacement(options UpdatePlacementOptions) (Placement, error) {
	replaceRange := strings.TrimSpace(options.ReplaceRange)
	if options.IndexProvided && options.Index <= 0 {
		return Placement{}, invalid("invalid --index (must be >= 1)")
	}

	if options.IndexProvided && replaceRange != "" {
		return Placement{}, invalid("--index cannot be combined with --replace-range")
	}

	if err := ValidateAnchor(options.Anchor); err != nil {
		return Placement{}, err
	}

	if options.Anchor.Text != "" && (options.IndexProvided || replaceRange != "") {
		return Placement{}, invalid("--at cannot be combined with --index or --replace-range")
	}

	target, replacing, err := ParseRange(replaceRange)
	if err != nil {
		return Placement{}, err
	}

	switch {
	case options.Anchor.Text != "":
		return Placement{Kind: PlacementAnchor, Anchor: options.Anchor}, nil
	case replacing:
		return Placement{Kind: PlacementRange, Range: target}, nil
	case options.IndexProvided:
		return Placement{Kind: PlacementIndex, Index: options.Index}, nil
	default:
		return Placement{Kind: PlacementEnd}, nil
	}
}

func PlanInsertPlacement(options InsertPlacementOptions) (Placement, error) {
	if options.Index != nil && *options.Index < 1 {
		return Placement{}, invalid("--index must be >= 1 (index 0 is reserved)")
	}

	if err := ValidateAnchor(options.Anchor); err != nil {
		return Placement{}, err
	}

	if options.Anchor.Text != "" && options.Index != nil {
		return Placement{}, invalid("--at and --index are mutually exclusive")
	}

	return insertPlacement(options.Index, false, options.Anchor), nil
}

func PlanEndInsertPlacement(options EndInsertPlacementOptions) (Placement, error) {
	if options.AtEnd && options.Index != nil {
		return Placement{}, invalid("--at-end and --index are mutually exclusive")
	}

	if options.Index != nil && *options.Index < 1 {
		return Placement{}, invalid("--index must be >= 1 (index 0 is reserved)")
	}

	if err := ValidateAnchor(options.Anchor); err != nil {
		return Placement{}, err
	}

	if options.Anchor.Text != "" && (options.AtEnd || options.Index != nil) {
		return Placement{}, invalid("--at cannot be combined with --at-end or --index")
	}

	return insertPlacement(options.Index, options.AtEnd, options.Anchor), nil
}

func PlanRangePlacement(options RangePlacementOptions) (Placement, error) {
	if err := ValidateAnchor(options.Anchor); err != nil {
		return Placement{}, err
	}

	hasRange := options.Start != nil || options.End != nil
	if options.Anchor.Text != "" && hasRange {
		return Placement{}, invalid("--at cannot be combined with --start or --end")
	}

	if options.Anchor.Text == "" && (options.Start == nil || options.End == nil) {
		return Placement{}, invalid("provide --at or both --start and --end")
	}

	if options.Anchor.Text != "" {
		return Placement{Kind: PlacementAnchor, Anchor: options.Anchor}, nil
	}

	if *options.Start < 1 {
		return Placement{}, invalid("--start must be >= 1")
	}

	if *options.End <= *options.Start {
		return Placement{}, invalid("--end must be greater than --start")
	}

	return Placement{
		Kind:  PlacementRange,
		Range: Range{Start: *options.Start, End: *options.End},
	}, nil
}

func ResolvePlacement(placement Placement, facts PlacementFacts) (ResolvedPlacement, error) {
	switch placement.Kind {
	case PlacementEnd:
		return ResolvedPlacement{
			Index: AppendIndex(facts.EndIndex),
			TabID: facts.TabID,
		}, nil
	case PlacementIndex:
		return ResolvedPlacement{
			Index: placement.Index,
			TabID: facts.TabID,
		}, nil
	case PlacementRange:
		target := placement.Range

		return ResolvedPlacement{
			Index: target.Start,
			Range: &target,
			TabID: facts.TabID,
		}, nil
	case PlacementAnchor:
		if facts.Anchor == nil {
			return ResolvedPlacement{}, fmt.Errorf("resolve anchor placement: %w", errMissingAnchorMatch)
		}

		target := Range{
			Start: facts.Anchor.StartIndex,
			End:   facts.Anchor.EndIndex,
		}

		return ResolvedPlacement{
			Index:              target.Start,
			Range:              &target,
			TabID:              facts.Anchor.TabID,
			RequiredRevisionID: facts.RequiredRevisionID,
			Anchored:           true,
			InTable:            facts.Anchor.InTable,
		}, nil
	default:
		return ResolvedPlacement{}, fmt.Errorf(
			"resolve placement kind %d: %w",
			placement.Kind,
			errUnsupportedPlacementKind,
		)
	}
}

func AppendIndex(endIndex int64) int64 {
	if endIndex > 1 {
		return endIndex - 1
	}

	return 1
}

func insertPlacement(index *int64, atEnd bool, anchor AnchorOptions) Placement {
	switch {
	case anchor.Text != "":
		return Placement{Kind: PlacementAnchor, Anchor: anchor}
	case atEnd || index == nil:
		return Placement{Kind: PlacementEnd}
	default:
		return Placement{Kind: PlacementIndex, Index: *index}
	}
}
