package cmd

import (
	"bytes"
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripYAMLFrontmatter removes a leading YAML frontmatter block: the file must
// begin (after optional UTF-8 BOM) with a line that trims to "---", followed by
// a later line that trims to "---". If no closing delimiter exists, b is
// returned unchanged.
func stripYAMLFrontmatter(b []byte) []byte {
	orig := b
	b = bytes.TrimPrefix(b, utf8BOM)
	firstLine, rest, ok := cutMarkdownLine(b)
	if !ok || !isYAMLFrontmatterDelimiter(firstLine) {
		return orig
	}

	for len(rest) > 0 {
		var line []byte
		line, rest, ok = cutMarkdownLine(rest)
		if isYAMLFrontmatterDelimiter(line) {
			if ok {
				return rest
			}
			return nil
		}
		if !ok {
			break
		}
	}
	return orig
}

func cutMarkdownLine(b []byte) (line []byte, rest []byte, ok bool) {
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return bytes.TrimSuffix(b, []byte{'\r'}), nil, false
	}
	return bytes.TrimSuffix(b[:i], []byte{'\r'}), b[i+1:], true
}

func isYAMLFrontmatterDelimiter(line []byte) bool {
	return string(bytes.TrimSpace(line)) == literalMarkdownTripleDash
}
