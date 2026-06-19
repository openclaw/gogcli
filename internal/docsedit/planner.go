package docsedit

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"

	"google.golang.org/api/docs/v1"

	"github.com/steipete/gogcli/internal/docsformat"
)

type ValidationError string

func (e ValidationError) Error() string {
	return string(e)
}

type Range struct {
	Start int64
	End   int64
}

type WriteOptions struct {
	EndIndex    int64
	InsertIndex int64
	Text        string
	TabID       string
	Append      bool
	Format      docsformat.Options
}

func BuildWriteRequests(options WriteOptions) ([]*docs.Request, error) {
	requests := make([]*docs.Request, 0, 3)

	if !options.Append {
		deleteEnd := options.EndIndex - 1
		if deleteEnd > 1 {
			requests = append(requests, BuildDeleteRequest(Range{Start: 1, End: deleteEnd}, options.TabID))
		}
	}

	requests = append(requests, BuildInsertRequest(options.Text, options.InsertIndex, options.TabID))
	if options.Format.Any() {
		format := options.Format
		if !options.Append && createsParagraphBullets(format) && hasParagraphStyle(format) {
			format.PostBulletParagraphStart = options.InsertIndex
			format.PostBulletParagraphEnd = options.InsertIndex + utf16Length(options.Text) - leadingParagraphTabs(options.Text)
		}

		formatRequests, err := docsformat.BuildRequests(
			format,
			options.InsertIndex,
			options.InsertIndex+utf16Length(options.Text),
			options.TabID,
		)
		if err != nil {
			return nil, fmt.Errorf("build format requests: %w", err)
		}

		requests = append(requests, formatRequests...)
	}

	return requests, nil
}

func createsParagraphBullets(options docsformat.Options) bool {
	return options.Bullets || options.Ordered || strings.TrimSpace(options.BulletPreset) != ""
}

func hasParagraphStyle(options docsformat.Options) bool {
	return strings.TrimSpace(options.Alignment) != "" || options.LineSpacing != 0 || options.HeadingLevel != nil ||
		strings.TrimSpace(options.NamedStyle) != "" || options.IndentStart != nil || options.IndentFirstLine != nil ||
		options.IndentEnd != nil || options.SpaceAbove != nil || options.SpaceBelow != nil ||
		options.KeepWithNext != nil || options.KeepLinesTogether != nil
}

func leadingParagraphTabs(text string) int64 {
	atParagraphStart := true
	count := int64(0)

	for _, r := range text {
		if r == '\n' {
			atParagraphStart = true
			continue
		}

		if atParagraphStart && r == '\t' {
			count++
			continue
		}
		atParagraphStart = false
	}

	return count
}

func BuildUpdateRequests(text string, insertIndex int64, tabID string, replace *Range) []*docs.Request {
	requests := make([]*docs.Request, 0, 2)
	if replace != nil {
		requests = append(requests, BuildDeleteRequest(*replace, tabID))
	}

	return append(requests, BuildInsertRequest(text, insertIndex, tabID))
}

func BuildInsertRequest(text string, index int64, tabID string) *docs.Request {
	return &docs.Request{InsertText: &docs.InsertTextRequest{
		Location: &docs.Location{Index: index, TabId: tabID},
		Text:     text,
	}}
}

func BuildDeleteRequest(target Range, tabID string) *docs.Request {
	return &docs.Request{DeleteContentRange: &docs.DeleteContentRangeRequest{
		Range: &docs.Range{StartIndex: target.Start, EndIndex: target.End, TabId: tabID},
	}}
}

func ParseRange(value string) (Range, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Range{}, false, nil
	}

	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return Range{}, false, invalid("invalid --replace-range (expected START:END)")
	}

	start, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 1 {
		return Range{}, false, invalid("invalid --replace-range start (must be >= 1)")
	}

	end, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil || end <= start {
		return Range{}, false, invalid("invalid --replace-range end (must be greater than start)")
	}

	return Range{Start: start, End: end}, true, nil
}

func utf16Length(value string) int64 {
	return int64(len(utf16.Encode([]rune(value))))
}

func invalid(message string) error {
	return ValidationError(message)
}
