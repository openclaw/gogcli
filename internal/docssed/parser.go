// Package docssed parses sed-style Google Docs mutation programs.
//
//nolint:err113,wsl_v5 // Parser errors include the exact invalid syntax for CLI diagnostics.
package docssed

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const backrefPlaceholder = "\x00BACKREF_"

// Command identifies the sed operation represented by an expression.
type Command byte

const (
	CommandSubstitute    Command = 0
	CommandDelete        Command = 'd'
	CommandAppend        Command = 'a'
	CommandInsert        Command = 'i'
	CommandTransliterate Command = 'y'
)

// Address targets one paragraph or an inclusive paragraph range.
type Address struct {
	Start    int
	End      int
	HasRange bool
}

// Expression is the provider-independent core AST for one sed operation.
type Expression struct {
	Pattern     string
	Replacement string
	Global      bool
	NthMatch    int
	Command     Command
	Address     *Address

	Cell        *CellReference
	Table       *TableReference
	Image       *ImageReference
	Brace       *BraceExpression
	BraceSpans  []*BraceSpan
	TableCreate *TableCreateSpec
}

// Program is an ordered sequence of sed expressions.
type Program struct {
	Expressions []Expression
}

var sedBackrefRE = regexp.MustCompile(`\\(\d)`)

var backrefReplacer = strings.NewReplacer(func() []string {
	var pairs []string
	for digit := 1; digit <= 9; digit++ {
		pairs = append(pairs, backrefPlaceholder+strconv.Itoa(digit)+"\x00", fmt.Sprintf("${%d}", digit))
	}
	return pairs
}()...)

// Parse parses one sed expression into a program.
func Parse(raw string) (Program, error) {
	expression, err := ParseExpression(raw)
	if err != nil {
		return Program{}, err
	}
	return Program{Expressions: []Expression{expression}}, nil
}

// ParseExpression parses substitution, delete, append, insert, transliterate, and addressed forms.
func ParseExpression(raw string) (Expression, error) {
	if raw == "" {
		return Expression{}, fmt.Errorf("empty expression")
	}

	address, remaining, err := ParseAddress(raw)
	if err != nil {
		return Expression{}, err
	}
	if address != nil && remaining == "" {
		return Expression{}, fmt.Errorf("address without command: %q", raw)
	}
	if address != nil {
		expression, parseErr := parseAddressedExpression(remaining)
		if parseErr != nil {
			return Expression{}, parseErr
		}
		expression.Address = address
		return expression, nil
	}

	return parseExpressionInner(raw)
}

func parseAddressedExpression(raw string) (Expression, error) {
	if len(raw) >= 1 {
		switch raw[0] {
		case byte(CommandDelete):
			if len(raw) == 1 {
				return Expression{Command: CommandDelete}, nil
			}
			if len(raw) >= 2 && !isAlphanumeric(raw[1]) {
				return ParseDelete(raw)
			}
		case byte(CommandAppend):
			if len(raw) >= 2 && !isAlphanumeric(raw[1]) {
				return parseAddressedInsertAppend(raw, CommandAppend)
			}
		case byte(CommandInsert):
			if len(raw) >= 2 && !isAlphanumeric(raw[1]) {
				return parseAddressedInsertAppend(raw, CommandInsert)
			}
		}
	}
	return parseExpressionInner(raw)
}

func parseExpressionInner(raw string) (Expression, error) {
	if raw == "" {
		return Expression{}, fmt.Errorf("empty expression")
	}
	if len(raw) >= 2 && !isAlphanumeric(raw[1]) {
		switch raw[0] {
		case byte(CommandDelete):
			return ParseDelete(raw)
		case byte(CommandAppend):
			return ParseInsertAppend(raw, CommandAppend)
		case byte(CommandInsert):
			return ParseInsertAppend(raw, CommandInsert)
		case byte(CommandTransliterate):
			return ParseTransliterate(raw)
		}
	}
	return ParseSubstitution(raw)
}

// ParseSubstitution parses an s/pattern/replacement/flags expression.
func ParseSubstitution(raw string) (Expression, error) {
	if len(raw) < 4 || raw[0] != 's' {
		return Expression{}, fmt.Errorf("invalid sed expression (expected s/pattern/replacement/[flags])")
	}

	delimiter := raw[1]
	parts := splitByDelimiter(raw[2:], delimiter)
	if len(parts) < 2 {
		return Expression{}, fmt.Errorf("invalid sed expression (missing replacement)")
	}
	flags := flagsFromParts(parts, 2)
	replacement := normalizeReplacement(parts[1])
	return Expression{
		Pattern:     applyRegexFlags(parts[0], flags),
		Replacement: replacement,
		Global:      strings.Contains(flags, "g"),
		NthMatch:    extractNumber(flags),
		Command:     CommandSubstitute,
	}, nil
}

func normalizeReplacement(replacement string) string {
	replacement = sedBackrefRE.ReplaceAllString(replacement, backrefPlaceholder+"${1}\x00")

	var processed strings.Builder
	for index := 0; index < len(replacement); index++ {
		switch {
		case replacement[index] == '\\' && index+1 < len(replacement):
			next := replacement[index+1]
			switch next {
			case '$':
				processed.WriteString("$$")
				index++
			case '.', '^', '[', ']', '(', ')', '{', '}', '+', '?', '|':
				processed.WriteByte(next)
				index++
			case '&':
				processed.WriteString("\x00LITAMP\x00")
				index++
			case '\\':
				processed.WriteByte('\\')
				index++
			default:
				processed.WriteByte('\\')
			}
		case replacement[index] == '$':
			switch {
			case index+1 < len(replacement) && replacement[index+1] == '$':
				processed.WriteString("$$")
				index++
			case index+1 < len(replacement) && replacement[index+1] >= '1' && replacement[index+1] <= '9':
				processed.WriteString(backrefPlaceholder)
				processed.WriteByte(replacement[index+1])
				processed.WriteByte('\x00')
				index++
			case index+1 < len(replacement) && replacement[index+1] == '{':
				processed.WriteByte('$')
			default:
				processed.WriteString("$$")
			}
		default:
			processed.WriteByte(replacement[index])
		}
	}

	replacement = backrefReplacer.Replace(processed.String())
	replacement = strings.ReplaceAll(replacement, "&", "${0}")
	return strings.ReplaceAll(replacement, "\x00LITAMP\x00", "&")
}

// ParseDelete parses a d/pattern/flags expression.
func ParseDelete(raw string) (Expression, error) {
	if len(raw) < 3 || raw[0] != byte(CommandDelete) {
		return Expression{}, fmt.Errorf("invalid delete command (expected d/pattern/)")
	}
	parts := splitByDelimiter(raw[2:], raw[1])
	if len(parts) < 1 || parts[0] == "" {
		return Expression{}, fmt.Errorf("invalid delete command (empty pattern)")
	}
	return Expression{
		Pattern: applyRegexFlags(parts[0], flagsFromParts(parts, 1)),
		Command: CommandDelete,
	}, nil
}

// ParseInsertAppend parses an append or insert expression with a search pattern.
func ParseInsertAppend(raw string, command Command) (Expression, error) {
	if command != CommandAppend && command != CommandInsert {
		return Expression{}, fmt.Errorf("invalid insert/append command: %q", command)
	}
	if len(raw) < 3 || raw[0] != byte(command) {
		return Expression{}, fmt.Errorf("invalid %c command", command)
	}
	parts := splitByDelimiter(raw[2:], raw[1])
	if len(parts) < 2 {
		return Expression{}, fmt.Errorf("invalid %c command (expected %c/pattern/text/)", command, command)
	}
	return Expression{
		Pattern:     applyRegexFlags(parts[0], flagsFromParts(parts, 2)),
		Replacement: parts[1],
		Command:     command,
	}, nil
}

func parseAddressedInsertAppend(raw string, command Command) (Expression, error) {
	if len(raw) < 3 || raw[0] != byte(command) {
		return Expression{}, fmt.Errorf("invalid %c command", command)
	}
	parts := splitByDelimiter(raw[2:], raw[1])
	if len(parts) < 1 || parts[0] == "" {
		return Expression{}, fmt.Errorf("invalid addressed %c command (expected %c/text/)", command, command)
	}
	return Expression{Replacement: parts[0], Command: command}, nil
}

// ParseTransliterate parses a y/source/destination expression.
func ParseTransliterate(raw string) (Expression, error) {
	if len(raw) < 3 || raw[0] != byte(CommandTransliterate) {
		return Expression{}, fmt.Errorf("invalid transliterate command (expected y/source/dest/)")
	}
	parts := splitByDelimiter(raw[2:], raw[1])
	if len(parts) < 2 {
		return Expression{}, fmt.Errorf("invalid transliterate command (expected y/source/dest/)")
	}
	source := parts[0]
	destination := parts[1]
	if len([]rune(source)) != len([]rune(destination)) {
		return Expression{}, fmt.Errorf(
			"transliterate: source and dest must have same length (%d vs %d)",
			len([]rune(source)),
			len([]rune(destination)),
		)
	}
	if source == "" {
		return Expression{}, fmt.Errorf("transliterate: empty source")
	}
	return Expression{
		Pattern:     source,
		Replacement: destination,
		Command:     CommandTransliterate,
	}, nil
}

// ParseAddress removes an optional paragraph address prefix.
func ParseAddress(raw string) (*Address, string, error) {
	if raw == "" {
		return nil, raw, nil
	}
	if raw[0] == '$' {
		if len(raw) == 1 {
			return &Address{Start: -1}, "", nil
		}
		remaining := raw[1:]
		if remaining[0] == ',' {
			return nil, raw, fmt.Errorf("invalid address: $ cannot be range start (use N,$ instead)")
		}
		return &Address{Start: -1}, remaining, nil
	}
	if raw[0] < '0' || raw[0] > '9' {
		return nil, raw, nil
	}

	index := 0
	for index < len(raw) && raw[index] >= '0' && raw[index] <= '9' {
		index++
	}
	start, err := strconv.Atoi(raw[:index])
	if err != nil || start < 1 {
		return nil, raw, nil //nolint:nilerr // Overflow means the prefix is not a valid address.
	}
	remaining := raw[index:]
	if len(remaining) > 0 && remaining[0] == ',' {
		remaining = remaining[1:]
		if remaining == "" {
			return nil, raw, fmt.Errorf("invalid address: range missing end")
		}
		if remaining[0] == '$' {
			return &Address{Start: start, End: -1, HasRange: true}, remaining[1:], nil
		}
		endIndex := 0
		for endIndex < len(remaining) && remaining[endIndex] >= '0' && remaining[endIndex] <= '9' {
			endIndex++
		}
		if endIndex == 0 {
			return nil, raw, fmt.Errorf("invalid address: range end must be a number or $")
		}
		end, endErr := strconv.Atoi(remaining[:endIndex])
		if endErr != nil || end < 1 {
			return nil, raw, fmt.Errorf("invalid address: range end must be >= 1")
		}
		if end < start {
			return nil, raw, fmt.Errorf("invalid address: range end (%d) < start (%d)", end, start)
		}
		return &Address{Start: start, End: end, HasRange: true}, remaining[endIndex:], nil
	}
	return &Address{Start: start}, remaining, nil
}

// NthFlag returns the positive occurrence selector from substitution flags.
func NthFlag(raw string) int {
	if len(raw) < 4 || raw[0] != 's' {
		return 0
	}
	parts := splitByDelimiter(raw[2:], raw[1])
	return extractNumber(flagsFromParts(parts, 2))
}

func extractNumber(value string) int {
	if index := strings.Index(value, "{"); index >= 0 {
		value = value[:index]
	}
	start := -1
	for index, character := range value {
		if character >= '0' && character <= '9' {
			if start < 0 {
				start = index
			}
		} else if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0
	}
	end := start
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	number, err := strconv.Atoi(value[start:end])
	if err != nil || number <= 0 {
		return 0
	}
	return number
}

func applyRegexFlags(pattern, flags string) string {
	if index := strings.Index(flags, "{"); index >= 0 {
		flags = flags[:index]
	}
	if strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}
	if strings.Contains(flags, "m") {
		pattern = "(?m)" + pattern
	}
	return pattern
}

func splitByDelimiter(value string, delimiter byte) []string {
	var parts []string
	var current strings.Builder
	for index := 0; index < len(value); index++ {
		switch {
		case value[index] == '\\' && index+1 < len(value) && value[index+1] == delimiter:
			current.WriteByte(delimiter)
			index++
		case value[index] == delimiter:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(value[index])
		}
	}
	return append(parts, current.String())
}

func isAlphanumeric(value byte) bool {
	return (value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= '0' && value <= '9')
}

func flagsFromParts(parts []string, index int) string {
	if index < len(parts) {
		return parts[index]
	}
	return ""
}
