package csv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	errEmptyCSV            = errors.New("empty csv")
	errFileRequired        = errors.New("file is required")
	errInvalidToken        = errors.New("invalid replacement token")
	errInvalidFilterFormat = errors.New("invalid filter format")
	errInvalidFilterField  = errors.New("invalid filter field")
)

type FieldFilter struct {
	Field string
	Regex *regexp.Regexp
}

type Options struct {
	Fields   []string
	Match    []FieldFilter
	Skip     []FieldFilter
	SkipRows int
	MaxRows  int
}

type Row struct {
	Index  int
	Values map[string]string
}

func Process(path string, opts Options, fn func(Row) error) error {
	reader, closer, err := openCSV(path)
	if err != nil {
		return err
	}

	if closer != nil {
		defer closer.Close()
	}

	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read csv: %w", err)
	}

	if len(records) == 0 {
		return errEmptyCSV
	}

	headers := normalizeHeader(records[0])
	selected := normalizeFields(opts.Fields)

	processed := 0

	for i, row := range records[1:] {
		rowIndex := i + 1
		if opts.SkipRows > 0 && rowIndex <= opts.SkipRows {
			continue
		}

		values := mapRow(headers, row, selected)
		if !matchesAllFilters(values, opts.Match) {
			continue
		}

		if matchesAnyFilter(values, opts.Skip) {
			continue
		}

		processed++
		if opts.MaxRows > 0 && processed > opts.MaxRows {
			break
		}

		if err := fn(Row{Index: rowIndex, Values: values}); err != nil {
			return err
		}
	}

	return nil
}

func SubstituteArgs(args []string, row Row) ([]string, error) {
	out := make([]string, len(args))
	for i, arg := range args {
		sub, err := substituteArg(arg, row)
		if err != nil {
			return nil, err
		}
		out[i] = sub
	}

	return out, nil
}

func openCSV(path string) (*csv.Reader, io.Closer, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return nil, nil, errFileRequired
	}

	if trimmed == "-" {
		return csv.NewReader(os.Stdin), nil, nil
	}

	f, err := os.Open(trimmed) //nolint:gosec // G304: user-provided file path is intentional
	if err != nil {
		return nil, nil, fmt.Errorf("open csv: %w", err)
	}

	return csv.NewReader(f), f, nil
}

func normalizeHeader(header []string) []string {
	out := make([]string, len(header))
	for i, h := range header {
		out[i] = normalizeField(h)
	}

	return out
}

func normalizeFields(fields []string) map[string]struct{} {
	if len(fields) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		if f = normalizeField(f); f != "" {
			set[f] = struct{}{}
		}
	}

	return set
}

func normalizeField(field string) string {
	return strings.ToLower(strings.TrimSpace(field))
}

func mapRow(headers, row []string, allowed map[string]struct{}) map[string]string {
	values := make(map[string]string, len(headers))
	for i, key := range headers {
		if key == "" {
			continue
		}

		if allowed != nil {
			if _, ok := allowed[key]; !ok {
				continue
			}
		}

		if i >= len(row) {
			values[key] = ""
			continue
		}
		values[key] = strings.TrimSpace(row[i])
	}

	return values
}

func matchesAllFilters(values map[string]string, filters []FieldFilter) bool {
	if len(filters) == 0 {
		return true
	}

	for _, filter := range filters {
		value := values[filter.Field]
		if filter.Regex == nil {
			continue
		}

		if !filter.Regex.MatchString(value) {
			return false
		}
	}

	return true
}

func matchesAnyFilter(values map[string]string, filters []FieldFilter) bool {
	if len(filters) == 0 {
		return false
	}

	for _, filter := range filters {
		value := values[filter.Field]
		if filter.Regex == nil {
			continue
		}

		if filter.Regex.MatchString(value) {
			return true
		}
	}

	return false
}

func substituteArg(arg string, row Row) (string, error) {
	if strings.Contains(arg, "~~") {
		replaced, err := replaceDoubleTilde(arg, row)
		if err != nil {
			return "", err
		}

		return replaced, nil
	}

	if strings.HasPrefix(arg, "~") {
		field := normalizeField(strings.TrimPrefix(arg, "~"))
		if field == "" {
			return "", nil
		}

		return row.Values[field], nil
	}

	return arg, nil
}

func replaceDoubleTilde(input string, row Row) (string, error) {
	out := input
	for {
		start := strings.Index(out, "~~")
		if start == -1 {
			return out, nil
		}
		rest := out[start+2:]

		end := strings.Index(rest, "~~")
		if end == -1 {
			return out, nil
		}
		token := rest[:end]

		replacement, err := resolveToken(token, row)
		if err != nil {
			return "", err
		}
		out = out[:start] + replacement + rest[end+2:]
	}
}

func resolveToken(token string, row Row) (string, error) {
	if strings.Contains(token, "~!~") {
		parts := strings.Split(token, "~!~")
		if len(parts) != 3 {
			return "", fmt.Errorf("%w: %s", errInvalidToken, token)
		}
		field := normalizeField(parts[0])
		pattern := parts[1]
		repl := parts[2]
		value := row.Values[field]

		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("invalid regex %q: %w", pattern, err)
		}

		return re.ReplaceAllString(value, repl), nil
	}

	field := normalizeField(token)

	return row.Values[field], nil
}

func ParseFieldFilters(inputs []string) ([]FieldFilter, error) {
	filters := make([]FieldFilter, 0, len(inputs))
	for _, item := range inputs {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%w: %q (expected FIELD:REGEX)", errInvalidFilterFormat, item)
		}

		field := normalizeField(parts[0])
		if field == "" {
			return nil, fmt.Errorf("%w: %q (missing field)", errInvalidFilterField, item)
		}

		re, err := regexp.Compile(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid filter regex %q: %w", parts[1], err)
		}

		filters = append(filters, FieldFilter{Field: field, Regex: re})
	}

	return filters, nil
}
