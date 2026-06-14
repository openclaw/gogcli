package cmd

import "testing"

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
