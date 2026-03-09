package cmd

import (
	"testing"
)

// FuzzParseSedExpr fuzzes the sed expression parser with arbitrary input.
// It ensures no panics on malformed expressions.
func FuzzParseSedExpr(f *testing.F) {
	// Seed corpus with valid and edge-case expressions
	seeds := []string{
		"s/hello/world/",
		"s/hello/world/g",
		"s/foo/**bar**/",
		"s/x/`code`/",
		"s/a/[link](https://example.com)/",
		"s/a/# heading/",
		"s/a/- bullet/",
		"s/a/1. numbered/",
		"s/a/+ checkbox/",
		"s/a/~~strike~~/",
		"s/a/__underline__/",
		"s/(a)(b)/$2$1/",
		"s/match/**&**/",
		"s/price/$$49.99/",
		"s/a/$$100/",
		"s/a/\\*escaped\\*/",
		"s/a/\\/path/",
		"s///",
		"s/a//",
		"s//b/",
		"s/a/b/xyz",
		"not a sed expression",
		"",
		"s",
		"s/",
		"s//",
		"s/a",
		"s/a/",
		"s/a/b",
		`s/a\/b/c\/d/`,
		"s/[unclosed/repl/",
		"s/(unmatched/repl/",
		"s/a/$$$$49/",
		"s/a/\\&literal/",
		"s/a/&whole/g",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		// Must not panic
		parseSedExpr(input)
	})
}

// FuzzParseSedExprWithCell fuzzes the cell-aware sed expression parser.
func FuzzParseSedExprWithCell(f *testing.F) {
	seeds := []string{
		"s/|1|[1,1]/hello/",
		"s/|2|[3,4]/**bold**/",
		"s/|1|[*,1]/all rows/",
		"s/|1|[1,*]/all cols/",
		"s/|3|[+1,1]//",
		"s/|3|[1,+1]//",
		"s/|1|[row:+2]//",
		"s/|1|[col:3]//",
		"s/|1|[1,1:2,3]/merge/",
		"s/|1|[A1]/hello/",
		"s/|1|[AB99]/data/",
		"s/|0|[1,1]/bad/",
		"s/|-1|[1,1]/bad/",
		"s/|1|[0,0]/bad/",
		"s/|1|[$+,1]//",
		"s/|1|[row:$+]//",
		"s/hello/world/",
		"s/|x|[1,1]/bad/",
		"",
		"s/|1|/delete table/",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseSedExprWithCell(input)
	})
}

// FuzzParseTableFromPipes fuzzes the markdown pipe-table parser.
func FuzzParseTableFromPipes(f *testing.F) {
	seeds := []string{
		"| A | B |\n| C | D |",
		"| **Bold** | *Italic* |\n| Data | More |",
		"|---|---|\n| A | B |",
		"| A | B |\n|---|---|\n| C | D |",
		"| Single |",
		"||",
		"| | |",
		"|A|B|C|D|E|F|G|H|I|J|",
		"not a table",
		"",
		"|",
		"|\n|",
		"| A |\n",
		"| A | B |\n| C |",
		"| **A** | `code` | [link](url) |\n| ~~strike~~ | __under__ | ***bolditalic*** |",
		"| Has\\nnewline | B |\n| C | D |",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseTableFromPipes(input)
	})
}

// FuzzParseTableCreate fuzzes the explicit table creation parser (|RxC| syntax).
func FuzzParseTableCreate(f *testing.F) {
	seeds := []string{
		"|3x4|",
		"|1x1|",
		"|10x10|",
		"|0x0|",
		"|3x|",
		"|x3|",
		"||",
		"|abc|",
		"3x4",
		"|3x4",
		"3x4|",
		"",
		"|100x100|",
		"|-1x3|",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseTableCreate(input)
	})
}

// FuzzParseImageSyntax fuzzes the markdown image syntax parser.
func FuzzParseImageSyntax(f *testing.F) {
	seeds := []string{
		"![alt](https://example.com/image.png)",
		"![](https://example.com/image.png)",
		`![alt](https://example.com/image.png "title")`,
		"![alt](url =100x200)",
		"![alt](url =100x)",
		"![alt](url =x200)",
		"![alt](url width=100 height=200)",
		"![alt](url width=100px)",
		"not an image",
		"",
		"![",
		"![]()",
		"![](",
		"![alt]",
		"![alt](",
		"![alt](url =0x0)",
		"![alt](url =-1x-1)",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseImageSyntax(input)
	})
}

// FuzzParseTableCellRef fuzzes the table cell reference parser.
func FuzzParseTableCellRef(f *testing.F) {
	seeds := []string{
		"|1|[1,1]",
		"|1|[*,1]",
		"|1|[1,*]",
		"|2|[+1,1]",
		"|2|[1,+1]",
		"|1|[row:+2]",
		"|1|[row:3]",
		"|1|[col:+1]",
		"|1|[col:2]",
		"|1|[row:$+]",
		"|1|[col:$+]",
		"|1|[1,1:2,3]",
		"|1|[A1]",
		"|1|[Z99]",
		"|1|[$+,1]",
		"",
		"|1|",
		"|1|[",
		"|1|[]",
		"|1|[,]",
		"|x|[1,1]",
		"||[1,1]",
		"|1|[abc]",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseTableCellRef(input)
	})
}

// FuzzParseFullExpr fuzzes the unified expression parser (s/d/a/i/y commands).
func FuzzParseFullExpr(f *testing.F) {
	seeds := []string{
		"s/hello/world/",
		"s/foo/bar/g",
		"s/foo/bar/2",
		"s/foo/bar/gim3",
		"d/pattern/",
		"d/pattern/i",
		"a/match/text/",
		"a/match/text/im",
		"i/match/text/",
		"y/abc/xyz/",
		"y/aeiou/AEIOU/",
		"",
		"x",
		"s",
		"d",
		"data",
		"d/",
		"d//",
		"a/x/",
		"y/ab/x/",
		"y//x/",
		"s/|1|[1,1]/hello/",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseFullExpr(input)
	})
}

// FuzzParseDCommand fuzzes the delete command parser.
func FuzzParseDCommand(f *testing.F) {
	seeds := []string{"d/foo/", "d/foo/i", "d/foo/m", "d//", "d", "d/\\//"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		parseDCommand(input)
	})
}

// FuzzParseYCommand fuzzes the transliterate command parser.
func FuzzParseYCommand(f *testing.F) {
	seeds := []string{"y/abc/xyz/", "y/a/b/", "y//x/", "y/ab/x/", "y", "y/abc/"}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		parseYCommand(input)
	})
}

// FuzzParseMarkdownReplacement fuzzes the markdown replacement string parser.
func FuzzParseMarkdownReplacement(f *testing.F) {
	seeds := []string{
		"**bold**",
		"*italic*",
		"***bolditalic***",
		"~~strike~~",
		"`code`",
		"__underline__",
		"[text](url)",
		"# Heading 1",
		"## Heading 2",
		"### Heading 3",
		"- bullet",
		"1. numbered",
		"+ checkbox",
		"plain text",
		"",
		"**",
		"*",
		"~~",
		"`",
		"__",
		"**unclosed",
		"*unclosed",
		"[unclosed](",
		"[text]()",
		"[](url)",
		// New formats
		"---", "***", "___",
		"> blockquote text",
		"```\ncode\n```",
		"```go\nfunc main() {}\n```",
		"  - nested bullet",
		"    - double nested",
		"  1. nested numbered",
		"^{superscript}",
		"~{subscript}",
		"E = mc^{2}",
		"H~{2}O",
		"{super=TM}",
		"{sub=2}",
		"E = mc{super=2}",
		"H{sub=2}O",
		"[^footnote text]",
		"[^]",
		">",
		"```",
		"^{}",
		"~{}",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		parseMarkdownReplacement(input)
	})
}
