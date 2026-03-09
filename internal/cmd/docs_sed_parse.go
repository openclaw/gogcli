package cmd

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const backrefPlaceholder = "\x00BACKREF_"

// sedBackrefRe matches sed-style backreferences (\1 through \9).
var sedBackrefRe = regexp.MustCompile(`\\(\d)`)

var backrefReplacer = strings.NewReplacer(func() []string {
	var pairs []string
	for d := 1; d <= 9; d++ {
		pairs = append(pairs, backrefPlaceholder+strconv.Itoa(d)+"\x00", fmt.Sprintf("${%d}", d))
	}
	return pairs
}()...)

// parseSedExpr parses a sed expression (s/pattern/replacement/flags) into its components.
// It handles escaped delimiters, backref conversion ($N), and & whole-match expansion.
func parseSedExpr(expr string) (pattern, replacement string, global bool, err error) {
	if len(expr) < 4 || expr[0] != 's' {
		return "", "", false, fmt.Errorf("invalid sed expression (expected s/pattern/replacement/[flags])")
	}

	delim := expr[1]

	// Split respecting escaped delimiters (\/ when delim is /)
	parts := splitByDelim(expr[2:], delim)

	if len(parts) < 2 {
		return "", "", false, fmt.Errorf("invalid sed expression (missing replacement)")
	}

	pattern = parts[0]
	replacement = parts[1]

	// Check flags
	flags := flagsFromParts(parts, 2)
	global = strings.Contains(flags, "g")
	pattern = applyRegexFlags(pattern, flags)

	// Process replacement escapes and backreferences.
	// In sed replacements: \1-\9 are backrefs, \$ is literal $, \. is literal ., \\ is literal \, etc.
	// In Go regex replacements: $1/${1} are backrefs, $$ is literal $.

	// Step 1: Convert sed backrefs \1-\9 to placeholders
	replacement = sedBackrefRe.ReplaceAllString(replacement, backrefPlaceholder+"${1}\x00")

	// Step 2: Process replacement string — unescape sed escapes and handle $ for Go regex.
	// In one pass: \$ → literal dollar (escaped as $$ for Go), \. → ., \\ → \,
	// $N → backref placeholder, other $ → $$ (literal for Go).
	// Preserve \n, \t for later processing.
	var processed strings.Builder
	for i := 0; i < len(replacement); i++ {
		switch {
		case replacement[i] == '\\' && i+1 < len(replacement):
			next := replacement[i+1]
			switch next {
			case '$':
				// \$ in sed replacement = literal $ → escape as $$ for Go regex
				processed.WriteString("$$")
				i++
			case '.', '^', '[', ']', '(', ')', '{', '}', '+', '?', '|':
				// Unescape regex metacharacters to literals
				processed.WriteByte(next)
				i++
			case '&':
				// \& = literal & (not a whole-match backref) — use placeholder
				processed.WriteString("\x00LITAMP\x00")
				i++
			case '\\':
				processed.WriteByte('\\')
				i++
			default:
				// Preserve other escapes (\n, \t, etc.)
				processed.WriteByte('\\')
			}
		case replacement[i] == '$':
			switch {
			case i+1 < len(replacement) && replacement[i+1] == '$':
				// $$ in sed replacement = literal $ → escape as $$ for Go regex
				processed.WriteString("$$")
				i++ // skip second $
			case i+1 < len(replacement) && replacement[i+1] >= '1' && replacement[i+1] <= '9':
				// $N backref — convert to placeholder
				processed.WriteString(backrefPlaceholder)
				processed.WriteByte(replacement[i+1])
				processed.WriteByte('\x00')
				i++
			case i+1 < len(replacement) && replacement[i+1] == '{':
				// ${N} backref — pass through
				processed.WriteByte('$')
			default:
				// Literal $ not followed by digit or $ — escape for Go regex
				processed.WriteString("$$")
			}
		default:
			processed.WriteByte(replacement[i])
		}
	}
	replacement = processed.String()

	// Step 3: Restore backrefs from placeholders → Go-style ${N}
	replacement = backrefReplacer.Replace(replacement)

	// Step 4: Handle sed & (whole match) → Go's ${0}
	// Unescaped & means "whole match" in sed. \& was converted to \x00LITAMP\x00 in Step 2.
	replacement = strings.ReplaceAll(replacement, "&", "${0}")
	replacement = strings.ReplaceAll(replacement, "\x00LITAMP\x00", "&")

	return pattern, replacement, global, nil
}

// parseSedExprWithCell parses a sed expression and extracts any table cell reference from the pattern.
// parseTableRef checks if a pattern is a bare table reference like |1|, |2|, |-1|, |*|
// Returns (tableIndex, ok). tableIndex is 1-indexed, negative from end, 0 means |*| (all).
func parseTableRef(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 3 || s[0] != '|' || s[len(s)-1] != '|' {
		return 0, false
	}
	inner := s[1 : len(s)-1]
	// Don't match table creation specs (contain 'x')
	if strings.ContainsAny(inner, "xX") {
		return 0, false
	}
	if inner == "*" {
		return math.MinInt32, true // all tables
	}
	n, err := strconv.Atoi(inner)
	if err != nil {
		return 0, false
	}
	if n == 0 {
		return 0, false
	}
	return n, true
}

// Cell ref patterns: s/|1|[2,3]/replacement/ or s/|1|[A1]:subpattern/replacement/
func parseSedExprWithCell(expr string) (pattern, replacement string, global bool, cellRef *tableCellRef, err error) {
	pattern, replacement, global, err = parseSedExpr(expr)
	if err != nil {
		return
	}

	// Check if pattern is a table cell reference
	ref := parseTableCellRef(pattern)
	if ref != nil {
		cellRef = ref
		if ref.subPattern != "" {
			pattern = ref.subPattern
		} else {
			pattern = "" // whole cell replacement
		}
	}
	return
}

// parseMarkdownReplacement extracts text and formatting from markdown-style replacement.
// Supported standard CommonMark formats: **bold**, *italic*, ***bold+italic***,
// ~~strikethrough~~, `code`, [text](url), # through ###### headings,
// - bullets, 1. numbered, > blockquotes, --- hrules, ```codeblocks```, [^footnotes].
// Escape sequences: \*, \#, \~, \`, \-, \+, \\, \n.
// Returns: plain text, format strings (e.g., "bold", "heading2", "link:url").
func parseMarkdownReplacement(repl string) (text string, formats []string) {
	text = repl

	// Process escape sequences and restore on return
	text = escapeMarkdown(text)
	defer func() { text = unescapeMarkdown(text) }()

	// Horizontal rule (---, ***, ___) — must be exactly 3 of the same char
	// (not 4+ which could be bold/italic markers)
	trimmed := strings.TrimSpace(text)
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return "\n", []string{"hrule"}
	}

	// Fenced code block (```...```)
	if strings.HasPrefix(text, "```") && strings.HasSuffix(text, "```") && len(text) > 6 {
		inner := text[3 : len(text)-3]
		// Strip optional language hint on first line (e.g., ```go)
		if idx := strings.Index(inner, "\n"); idx >= 0 {
			inner = inner[idx+1:]
		}
		return inner, append(formats, "codeblock")
	}

	// Blockquote (> text)
	if strings.HasPrefix(text, "> ") {
		return text[2:], append(formats, "blockquote")
	}

	// Footnote [^text] — creates a footnote with the given text
	if strings.HasPrefix(text, "[^") && strings.HasSuffix(text, "]") && len(text) > 3 {
		footnoteText := text[2 : len(text)-1]
		return footnoteText, []string{"footnote"}
	}

	// Check for list prefixes with optional indentation for nesting
	// Detect indent level: 2 spaces = 1 level, 4 spaces = 2 levels, etc.
	indentLevel := 0
	listText := text
	for strings.HasPrefix(listText, "  ") {
		indentLevel++
		listText = listText[2:]
	}

	listFormat := ""
	switch {
	case strings.HasPrefix(listText, "- "):
		text = listText[2:]
		listFormat = "bullet"
	case strings.HasPrefix(listText, "* ") && !strings.HasSuffix(listText, "*"):
		text = listText[2:]
		listFormat = "bullet"
	case len(listText) > 2 && listText[0] >= '0' && listText[0] <= '9' && listText[1] == '.' && listText[2] == ' ':
		text = listText[3:]
		listFormat = "numbered"
	}

	if listFormat != "" {
		formats = append(formats, listFormat)
		if indentLevel > 0 {
			// Prepend tab characters for nesting. When CreateParagraphBullets
			// is applied, Google Docs converts leading \t into nesting levels.
			text = strings.Repeat("\t", indentLevel) + text
		}
	}

	// Bold+italic (***text***)
	if strings.HasPrefix(text, "***") && strings.HasSuffix(text, "***") && len(text) > 6 {
		return text[3 : len(text)-3], append(formats, "bold", "italic")
	}
	// Bold (**text**)
	if strings.HasPrefix(text, "**") && strings.HasSuffix(text, "**") && len(text) > 4 {
		return text[2 : len(text)-2], append(formats, "bold")
	}
	// Italic (*text*) - only if not already parsed as bullet
	if strings.HasPrefix(text, "*") && strings.HasSuffix(text, "*") && len(text) > 2 {
		return text[1 : len(text)-1], append(formats, "italic")
	}
	// Strikethrough (~~text~~)
	if strings.HasPrefix(text, "~~") && strings.HasSuffix(text, "~~") && len(text) > 4 {
		return text[2 : len(text)-2], append(formats, "strikethrough")
	}
	// Code (`text`)
	if strings.HasPrefix(text, "`") && strings.HasSuffix(text, "`") && len(text) > 2 {
		return text[1 : len(text)-1], append(formats, "code")
	}
	// Link [text](url) — can appear anywhere in the replacement
	if idx := strings.Index(text, "]("); idx > 0 && strings.HasPrefix(text, "[") {
		closeParen := strings.LastIndex(text, ")")
		if closeParen > idx+2 {
			linkText := text[1:idx]
			linkURL := text[idx+2 : closeParen]
			linkURL = strings.ReplaceAll(linkURL, "\\/", "/")
			return linkText, append(formats, "link:"+linkURL)
		}
	}
	// Headings (#text, ##text, etc)
	if strings.HasPrefix(text, "#") {
		level := 0
		for i := 0; i < len(text) && i < 6; i++ {
			if text[i] == '#' {
				level++
			} else {
				break
			}
		}
		if level > 0 && level <= 6 {
			stripped := strings.TrimPrefix(text[level:], " ")
			return stripped, append(formats, fmt.Sprintf("heading%d", level))
		}
	}

	return text, formats
}

// parseAddress strips an optional paragraph-number address prefix from a raw expression.
// Addresses are: N (single), N,M (range), $ (last), $-N (offset from last).
// Returns nil address and the original string if no address prefix found.
func parseAddress(raw string) (*sedAddress, string, error) {
	if len(raw) == 0 {
		return nil, raw, nil
	}

	// Check for $ (last paragraph) prefix
	if raw[0] == '$' {
		if len(raw) == 1 {
			return &sedAddress{Start: -1}, "", nil
		}
		remaining := raw[1:]
		// $,N range
		if remaining[0] == ',' {
			return nil, raw, fmt.Errorf("invalid address: $ cannot be range start (use N,$ instead)")
		}
		// $ followed by a command
		return &sedAddress{Start: -1}, remaining, nil
	}

	// Check for leading digits
	if raw[0] < '0' || raw[0] > '9' {
		return nil, raw, nil
	}

	// Parse the start number
	i := 0
	for i < len(raw) && raw[i] >= '0' && raw[i] <= '9' {
		i++
	}
	start, err := strconv.Atoi(raw[:i])
	if err != nil {
		return nil, raw, nil //nolint:nilerr // not an address, pass through
	}
	if start < 1 {
		return nil, raw, nil
	}

	remaining := raw[i:]

	// Check for comma (range)
	if len(remaining) > 0 && remaining[0] == ',' {
		remaining = remaining[1:]
		if len(remaining) == 0 {
			return nil, raw, fmt.Errorf("invalid address: range missing end")
		}
		// End can be $ or a number
		if remaining[0] == '$' {
			return &sedAddress{Start: start, End: -1, HasRange: true}, remaining[1:], nil
		}
		j := 0
		for j < len(remaining) && remaining[j] >= '0' && remaining[j] <= '9' {
			j++
		}
		if j == 0 {
			return nil, raw, fmt.Errorf("invalid address: range end must be a number or $")
		}
		end, endErr := strconv.Atoi(remaining[:j])
		if endErr != nil || end < 1 {
			return nil, raw, fmt.Errorf("invalid address: range end must be >= 1")
		}
		if end < start {
			return nil, raw, fmt.Errorf("invalid address: range end (%d) < start (%d)", end, start)
		}
		return &sedAddress{Start: start, End: end, HasRange: true}, remaining[j:], nil
	}

	// Single address — but only if followed by a command character, not more digits
	// that could be part of something else (like a pattern).
	// We need to distinguish "5d" (address 5 + delete) from "5" by itself.
	if len(remaining) == 0 {
		// Bare number with nothing after — treat as addressed bare command (needs a command)
		return &sedAddress{Start: start}, "", nil
	}

	return &sedAddress{Start: start}, remaining, nil
}

// parseFullExpr parses a raw expression string into a sedExpr, handling all command types
// (s//, d//, a//, i//, y//) and flags (g, i, m, N for nth occurrence).
// Supports optional paragraph-number address prefix: 5d, 3,7s/foo/bar/, $a/text/.
func parseFullExpr(raw string) (sedExpr, error) {
	if len(raw) == 0 {
		return sedExpr{}, fmt.Errorf("empty expression")
	}

	// Try to parse an address prefix
	addr, remaining, addrErr := parseAddress(raw)
	if addrErr != nil {
		return sedExpr{}, addrErr
	}

	// If we got an address, parse the remaining expression
	if addr != nil && remaining != "" {
		// Remaining starts with a command character
		var expr sedExpr
		var err error

		// Check for non-substitution commands
		if len(remaining) >= 1 {
			switch remaining[0] {
			case 'd':
				if len(remaining) == 1 {
					// Bare addressed delete: "5d" or "3,7d"
					expr = sedExpr{command: 'd'}
					expr.addr = addr
					return expr, nil
				}
				if len(remaining) >= 2 && !isAlphanumeric(remaining[1]) {
					expr, err = parseDCommand(remaining)
					if err != nil {
						return sedExpr{}, err
					}
					expr.addr = addr
					return expr, nil
				}
			case 'a':
				if len(remaining) >= 2 && !isAlphanumeric(remaining[1]) {
					expr, err = parseAddressedAICommand(remaining, 'a')
					if err != nil {
						return sedExpr{}, err
					}
					expr.addr = addr
					return expr, nil
				}
			case 'i':
				if len(remaining) >= 2 && !isAlphanumeric(remaining[1]) {
					expr, err = parseAddressedAICommand(remaining, 'i')
					if err != nil {
						return sedExpr{}, err
					}
					expr.addr = addr
					return expr, nil
				}
			}
		}

		// Otherwise parse as s// or other standard command
		expr, err = parseFullExprInner(remaining)
		if err != nil {
			return sedExpr{}, err
		}
		expr.addr = addr
		return expr, nil
	}

	// Address with no remaining command — bare addressed command
	if addr != nil && remaining == "" {
		return sedExpr{}, fmt.Errorf("address without command: %q", raw)
	}

	// No address — parse normally
	return parseFullExprInner(raw)
}

// parseFullExprInner is the original parseFullExpr logic, extracted so parseFullExpr
// can handle address prefixes before delegating.
func parseFullExprInner(raw string) (sedExpr, error) {
	if len(raw) == 0 {
		return sedExpr{}, fmt.Errorf("empty expression")
	}

	// Check for non-substitution commands: d, a, i, y
	// Only treat as command if followed by a non-alphanumeric delimiter (like /)
	if len(raw) >= 2 && !isAlphanumeric(raw[1]) {
		switch raw[0] {
		case 'd':
			return parseDCommand(raw)
		case 'a':
			return parseAICommand(raw, 'a')
		case 'i':
			return parseAICommand(raw, 'i')
		case 'y':
			return parseYCommand(raw)
		}
	}

	// Standard s// command
	pattern, replacement, global, cellRef, err := parseSedExprWithCell(raw)
	if err != nil {
		return sedExpr{}, err
	}

	expr := sedExpr{pattern: pattern, replacement: replacement, global: global, cellRef: cellRef}

	// Check for brace pattern-side addressing: {T=...} or {img=...}
	// This is SEDMAT v3.5 syntax for table/image addressing in the pattern position
	if cellRef == nil && strings.HasPrefix(pattern, "{") {
		remaining, tableRef, imgRef, braceErr := detectBracePattern(pattern)
		if braceErr != nil {
			return sedExpr{}, fmt.Errorf("brace pattern: %w", braceErr)
		}
		if tableRef != nil {
			// Bridge to existing table machinery
			braceTableToSedExpr(tableRef, &expr)
			expr.pattern = remaining
			// If this is a table creation spec, handle via replacement
			if tableRef.IsCreate {
				spec := braceTableToTableCreateSpec(tableRef)
				if spec != nil {
					// Convert to pipe-style for existing machinery
					if spec.header {
						expr.replacement = fmt.Sprintf("|%dx%d:header|", spec.rows, spec.cols)
					} else {
						expr.replacement = fmt.Sprintf("|%dx%d|", spec.rows, spec.cols)
					}
				}
			}
		}
		if imgRef != nil {
			// Bridge to existing image machinery via pattern
			imgPattern := braceImgToImageRefPattern(imgRef)
			if imgPattern != nil {
				switch {
				case imgPattern.AllImages:
					expr.pattern = "!(*)"
				case imgPattern.ByPosition:
					expr.pattern = fmt.Sprintf("!(%d)", imgPattern.Position)
				case imgPattern.ByAlt && imgPattern.AltRegex != nil:
					expr.pattern = fmt.Sprintf("![%s]", imgRef.Pattern)
				}
			}
		}
	}

	// Check if pattern is a bare table reference (|1|, |-1|, |*|) — legacy pipe syntax
	if expr.cellRef == nil && expr.tableRef == 0 {
		if tIdx, ok := parseTableRef(expr.pattern); ok {
			expr.tableRef = tIdx
			expr.pattern = ""
		}
	}

	// Extract nth-match flag from raw expression flags
	expr.nthMatch = parseNthFlag(raw)

	// Check for brace formatting in replacement (SEDMAT v3.5 syntax)
	if hasBraceFormatting(replacement) {
		cleanedText, spans := findBraceExprs(replacement)
		if len(spans) > 0 {
			expr.replacement = cleanedText
			expr.braceSpans = spans
			// If there's exactly one global span, use it as the main brace expr
			if len(spans) == 1 && spans[0].IsGlobal {
				expr.brace = spans[0].Expr
			} else {
				// Multiple spans or non-global: merge into one brace for global formatting
				expr.brace = mergeBraceSpans(spans)
			}

			// Handle {T=NxM} table creation in replacement position:
			// convert to pipe-style spec for existing table machinery.
			if expr.brace != nil && expr.brace.TableRef != "" {
				bt, btErr := parseBraceTableRef(expr.brace.TableRef)
				if btErr == nil && bt.IsCreate {
					spec := braceTableToTableCreateSpec(bt)
					if spec != nil {
						if spec.header {
							expr.replacement = fmt.Sprintf("|%dx%d:header|", spec.rows, spec.cols)
						} else {
							expr.replacement = fmt.Sprintf("|%dx%d|", spec.rows, spec.cols)
						}
						expr.brace = nil
						expr.braceSpans = nil
					}
				}
			}
		}
	}

	return expr, nil
}

// parseNthFlag extracts the Nth occurrence flag (e.g., "2" in s/foo/bar/2) from a raw expression.
func parseNthFlag(raw string) int {
	if len(raw) < 4 || raw[0] != 's' {
		return 0
	}
	parts := splitByDelim(raw[2:], raw[1])
	flags := flagsFromParts(parts, 2)
	return extractNumber(flags)
}

// extractNumber returns the first contiguous positive integer found in a string, or 0.
// Ignores content inside {attrs} blocks.
func extractNumber(s string) int {
	if idx := strings.Index(s, "{"); idx >= 0 {
		s = s[:idx]
	}
	// Find first digit run
	start := -1
	for i, c := range s {
		if c >= '0' && c <= '9' {
			if start < 0 {
				start = i
			}
		} else if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0
	}
	end := start
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(s[start:end])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// parseDCommand parses a delete command: d/pattern/ or d/pattern/flags
// Deletes all text matching the pattern (entire line containing match).
func parseDCommand(raw string) (sedExpr, error) {
	if len(raw) < 3 || raw[0] != 'd' {
		return sedExpr{}, fmt.Errorf("invalid delete command (expected d/pattern/)")
	}
	parts := splitByDelim(raw[2:], raw[1])
	if len(parts) < 1 || parts[0] == "" {
		return sedExpr{}, fmt.Errorf("invalid delete command (empty pattern)")
	}
	pattern := applyRegexFlags(parts[0], flagsFromParts(parts, 1))
	return sedExpr{pattern: pattern, command: 'd'}, nil
}

// parseAddressedAICommand parses append/insert when used with a paragraph address: Na/text/.
// Unlike pattern-matched a/i, addressed a/i takes a single field (the text to insert),
// since the address already specifies where.
func parseAddressedAICommand(raw string, cmd byte) (sedExpr, error) {
	if len(raw) < 3 || raw[0] != cmd {
		return sedExpr{}, fmt.Errorf("invalid %c command", cmd)
	}
	parts := splitByDelim(raw[2:], raw[1])
	if len(parts) < 1 || parts[0] == "" {
		return sedExpr{}, fmt.Errorf("invalid addressed %c command (expected %c/text/)", cmd, cmd)
	}
	return sedExpr{replacement: parts[0], command: cmd}, nil
}

// parseAICommand parses append/insert commands: a/pattern/text/ or i/pattern/text/
// 'a' appends text after the matched line; 'i' inserts text before the matched line.
func parseAICommand(raw string, cmd byte) (sedExpr, error) {
	if len(raw) < 3 || raw[0] != cmd {
		return sedExpr{}, fmt.Errorf("invalid %c command", cmd)
	}
	parts := splitByDelim(raw[2:], raw[1])
	if len(parts) < 2 {
		return sedExpr{}, fmt.Errorf("invalid %c command (expected %c/pattern/text/)", cmd, cmd)
	}
	pattern := applyRegexFlags(parts[0], flagsFromParts(parts, 2))
	return sedExpr{pattern: pattern, replacement: parts[1], command: cmd}, nil
}

// flagsFromParts returns the flags string from parts[idx] if it exists, or empty string.
func flagsFromParts(parts []string, idx int) string {
	if idx < len(parts) {
		return parts[idx]
	}
	return ""
}

// applyRegexFlags prepends Go regex flag syntax for i (case-insensitive) and m (multiline).
func applyRegexFlags(pattern, flags string) string {
	// Strip {attrs} block from flags before checking flag characters
	if idx := strings.Index(flags, "{"); idx >= 0 {
		flags = flags[:idx]
	}
	if strings.Contains(flags, "i") {
		pattern = "(?i)" + pattern
	}
	if strings.Contains(flags, "m") {
		pattern = "(?m)" + pattern
	}
	return pattern
}

// parseYCommand parses a transliterate command: y/source/dest/
// Each character in source is replaced by the corresponding character in dest.
func parseYCommand(raw string) (sedExpr, error) {
	if len(raw) < 3 || raw[0] != 'y' {
		return sedExpr{}, fmt.Errorf("invalid transliterate command (expected y/source/dest/)")
	}
	parts := splitByDelim(raw[2:], raw[1])
	if len(parts) < 2 {
		return sedExpr{}, fmt.Errorf("invalid transliterate command (expected y/source/dest/)")
	}
	source := parts[0]
	dest := parts[1]
	if len([]rune(source)) != len([]rune(dest)) {
		return sedExpr{}, fmt.Errorf("transliterate: source and dest must have same length (%d vs %d)", len([]rune(source)), len([]rune(dest)))
	}
	if source == "" {
		return sedExpr{}, fmt.Errorf("transliterate: empty source")
	}
	return sedExpr{pattern: source, replacement: dest, command: 'y'}, nil
}

// splitByDelim splits a string by an unescaped delimiter character.
func splitByDelim(s string, delim byte) []string {
	var parts []string
	var current strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == delim:
			current.WriteByte(delim)
			i++
		case s[i] == delim:
			parts = append(parts, current.String())
			current.Reset()
		default:
			current.WriteByte(s[i])
		}
	}
	parts = append(parts, current.String())
	return parts
}

// isAlphanumeric returns true if b is a letter or digit.
func isAlphanumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// (Legacy parseAttrsFromRaw, parseAttrs, sedAttrs removed — use brace syntax instead)
