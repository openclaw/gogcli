//nolint:wsl_v5 // Phase classification and index transforms stay adjacent.
package docssed

import "strings"

// TextPlan separates matched replacements by provider execution requirements.
type TextPlan struct {
	MatchCount int
	Images     []ImageMutation
	Footnotes  []FootnoteMutation
	TextEdits  []TextEdit
	Formatting []FormatIntent
}

// ImageMutation replaces one text range with an inline image.
type ImageMutation struct {
	StartIndex int64
	EndIndex   int64
	Image      *ImageSpec
}

// FootnoteMutation replaces one text range with a populated footnote.
type FootnoteMutation struct {
	StartIndex int64
	EndIndex   int64
	Text       string
}

// TextEdit describes one delete followed by an optional insertion.
type TextEdit struct {
	StartIndex     int64
	EndIndex       int64
	InsertText     string
	HorizontalRule bool
}

// FormatIntent describes formatting for newly inserted text.
type FormatIntent struct {
	StartIndex           int64
	EndIndex             int64
	StructuralStartIndex int64
	StructuralEndIndex   int64
	Formats              []string
	LeadingTab           bool
	Brace                *BraceExpression
	BraceSpans           []*BraceSpan
}

// PlanTextMutations converts match actions into deterministic execution phases.
func PlanTextMutations(actions []MatchAction) TextPlan {
	plan := TextPlan{MatchCount: len(actions)}
	classes := classifyTextMutations(actions)
	for _, action := range actions {
		replacement := action.Replacement
		switch {
		case hasReplacementFormat(replacement.Formats, "footnote"):
			start := action.StartIndex +
				deltaBefore(classes.images, action.StartIndex) +
				deltaBefore(classes.text, action.StartIndex)
			plan.Footnotes = append(plan.Footnotes, FootnoteMutation{
				StartIndex: start,
				EndIndex:   start + action.EndIndex - action.StartIndex,
				Text:       replacement.Text,
			})
		case replacement.Image != nil:
			plan.Images = append(plan.Images, ImageMutation{
				StartIndex: action.StartIndex,
				EndIndex:   action.EndIndex,
				Image:      replacement.Image,
			})
		default:
			plan.appendTextMutation(action, classes)
		}
	}
	return plan
}

func (p *TextPlan) appendTextMutation(action MatchAction, classes mutationClasses) {
	replacement := action.Replacement
	horizontalRule := hasReplacementFormat(replacement.Formats, "hrule")
	insertText := replacement.Text
	if horizontalRule {
		insertText = "\n"
	}
	editStart := action.StartIndex + deltaBefore(classes.images, action.StartIndex)
	p.TextEdits = append(p.TextEdits, TextEdit{
		StartIndex:     editStart,
		EndIndex:       editStart + action.EndIndex - action.StartIndex,
		InsertText:     insertText,
		HorizontalRule: horizontalRule,
	})
	if horizontalRule || insertText == "" ||
		(len(replacement.Formats) == 0 && replacement.Brace == nil) {
		return
	}

	formats := append([]string(nil), replacement.Formats...)
	if hasReplacementFormat(formats, "codeblock") {
		formats = append(formats, "code")
	}
	formatStart := editStart + deltaBefore(classes.text, action.StartIndex)
	structuralStart := formatStart + deltaBefore(classes.footnotes, action.StartIndex)
	insertLength := utf16Length(insertText)
	p.Formatting = append(p.Formatting, FormatIntent{
		StartIndex:           formatStart,
		EndIndex:             formatStart + insertLength,
		StructuralStartIndex: structuralStart,
		StructuralEndIndex:   structuralStart + insertLength,
		Formats:              formats,
		LeadingTab:           strings.HasPrefix(insertText, "\t"),
		Brace:                replacement.Brace,
		BraceSpans:           replacement.BraceSpans,
	})
}

type indexedChange struct {
	start     int64
	oldLength int64
	newLength int64
}

type mutationClasses struct {
	images    []indexedChange
	text      []indexedChange
	footnotes []indexedChange
}

func classifyTextMutations(actions []MatchAction) mutationClasses {
	classes := mutationClasses{}
	for _, action := range actions {
		replacement := action.Replacement
		change := indexedChange{
			start:     action.StartIndex,
			oldLength: action.EndIndex - action.StartIndex,
		}
		switch {
		case hasReplacementFormat(replacement.Formats, "footnote"):
			change.newLength = 1
			classes.footnotes = append(classes.footnotes, change)
		case replacement.Image != nil:
			change.newLength = 1
			classes.images = append(classes.images, change)
		default:
			if hasReplacementFormat(replacement.Formats, "hrule") {
				change.newLength = 1
			} else {
				change.newLength = utf16Length(replacement.Text)
			}
			classes.text = append(classes.text, change)
		}
	}
	return classes
}

func deltaBefore(changes []indexedChange, index int64) int64 {
	var delta int64
	for _, change := range changes {
		if change.start < index {
			delta += change.newLength - change.oldLength
		}
	}
	return delta
}

func hasReplacementFormat(formats []string, target string) bool {
	for _, format := range formats {
		if format == target {
			return true
		}
	}
	return false
}
