//nolint:wsl_v5 // Table-driven parser tests stay compact around assertions.
package docssed

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSubstitution(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		raw         string
		wantPattern string
		wantReplace string
		wantGlobal  bool
		wantNth     int
		wantError   string
	}{
		{name: "basic", raw: "s/foo/bar/", wantPattern: "foo", wantReplace: "bar"},
		{name: "flags", raw: "s/foo/bar/3gim", wantPattern: "(?m)(?i)foo", wantReplace: "bar", wantGlobal: true, wantNth: 3},
		{name: "alternate delimiter", raw: "s#foo#bar#g", wantPattern: "foo", wantReplace: "bar", wantGlobal: true},
		{name: "escaped delimiter", raw: `s/a\/b/c\/d/`, wantPattern: "a/b", wantReplace: "c/d"},
		{name: "capture backref", raw: `s/(foo)/\1bar/`, wantPattern: "(foo)", wantReplace: "${1}bar"},
		{name: "dollar backref", raw: `s/(foo)/$1bar/`, wantPattern: "(foo)", wantReplace: "${1}bar"},
		{name: "whole match", raw: `s/foo/&/`, wantPattern: "foo", wantReplace: "${0}"},
		{name: "literal ampersand", raw: `s/foo/\&/`, wantPattern: "foo", wantReplace: "&"},
		{name: "literal dollar", raw: `s/foo/\$/`, wantPattern: "foo", wantReplace: "$$"},
		{name: "empty replacement", raw: "s/foo//", wantPattern: "foo"},
		{name: "wrong command", raw: "x/foo/bar/", wantError: "invalid sed expression"},
		{name: "too short", raw: "s/", wantError: "invalid sed expression"},
		{name: "missing replacement", raw: "s/foo", wantError: "missing replacement"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseSubstitution(test.raw)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("error = %v, want containing %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSubstitution: %v", err)
			}
			if got.Pattern != test.wantPattern ||
				got.Replacement != test.wantReplace ||
				got.Global != test.wantGlobal ||
				got.NthMatch != test.wantNth {
				t.Fatalf("expression = %+v", got)
			}
		})
	}
}

func TestParseExpressionCommandsAndAddresses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want Expression
	}{
		{raw: "d/foo/i", want: Expression{Pattern: "(?i)foo", Command: CommandDelete}},
		{raw: "a/foo/after/", want: Expression{Pattern: "foo", Replacement: "after", Command: CommandAppend}},
		{raw: "i/foo/before/", want: Expression{Pattern: "foo", Replacement: "before", Command: CommandInsert}},
		{raw: "y/áéí/AEI/", want: Expression{Pattern: "áéí", Replacement: "AEI", Command: CommandTransliterate}},
		{raw: "5d", want: Expression{Command: CommandDelete, Address: &Address{Start: 5}}},
		{raw: "3,7s/old/new/g", want: Expression{
			Pattern: "old", Replacement: "new", Global: true, Address: &Address{Start: 3, End: 7, HasRange: true},
		}},
		{raw: "$a/last line/", want: Expression{Replacement: "last line", Command: CommandAppend, Address: &Address{Start: -1}}},
		{raw: "5d/foo/", want: Expression{Pattern: "foo", Command: CommandDelete, Address: &Address{Start: 5}}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.raw, func(t *testing.T) {
			t.Parallel()
			got, err := ParseExpression(test.raw)
			if err != nil {
				t.Fatalf("ParseExpression: %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("expression = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseExpressionErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "", want: "empty expression"},
		{raw: "5", want: "address without command"},
		{raw: "7,3d", want: "range end (3) < start (7)"},
		{raw: "3,", want: "range missing end"},
		{raw: "d//", want: "empty pattern"},
		{raw: "a/foo", want: "expected a/pattern/text/"},
		{raw: "y/abc/xy/", want: "same length"},
		{raw: "y//abc/", want: "same length"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.raw, func(t *testing.T) {
			t.Parallel()
			_, err := ParseExpression(test.raw)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestParseInsertAppendRejectsOtherCommands(t *testing.T) {
	t.Parallel()
	_, err := ParseInsertAppend("d/foo/bar/", CommandDelete)
	if err == nil || !strings.Contains(err.Error(), "invalid insert/append command") {
		t.Fatalf("error = %v", err)
	}
}

func TestParserHelpers(t *testing.T) {
	t.Parallel()
	numberTests := []struct {
		value string
		want  int
	}{
		{value: "", want: 0},
		{value: "g", want: 0},
		{value: "2", want: 2},
		{value: "g3", want: 3},
		{value: "2g", want: 2},
		{value: "10", want: 10},
		{value: "0", want: 0},
		{value: "-1", want: 1},
		{value: "12{b}", want: 12},
	}
	for _, test := range numberTests {
		if got := extractNumber(test.value); got != test.want {
			t.Fatalf("ExtractNumber(%q) = %d, want %d", test.value, got, test.want)
		}
	}
	if got := NthFlag("s/foo/bar/10g"); got != 10 {
		t.Fatalf("NthFlag = %d", got)
	}
	flagTests := []struct {
		flags string
		want  string
	}{
		{flags: "", want: "foo"},
		{flags: "i", want: "(?i)foo"},
		{flags: "m", want: "(?m)foo"},
		{flags: "im", want: "(?m)(?i)foo"},
		{flags: "gim", want: "(?m)(?i)foo"},
	}
	for _, test := range flagTests {
		if got := applyRegexFlags("foo", test.flags); got != test.want {
			t.Fatalf("ApplyRegexFlags(%q) = %q, want %q", test.flags, got, test.want)
		}
	}
	splitTests := []struct {
		value     string
		delimiter byte
		want      []string
	}{
		{value: "a/b/c", delimiter: '/', want: []string{"a", "b", "c"}},
		{value: `a\/b/c`, delimiter: '/', want: []string{"a/b", "c"}},
		{value: "//", delimiter: '/', want: []string{"", "", ""}},
		{value: "abc", delimiter: '/', want: []string{"abc"}},
		{value: "a|b|c", delimiter: '|', want: []string{"a", "b", "c"}},
	}
	for _, test := range splitTests {
		if got := splitByDelimiter(test.value, test.delimiter); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("SplitByDelimiter(%q) = %#v, want %#v", test.value, got, test.want)
		}
	}
	if !isAlphanumeric('A') || !isAlphanumeric('7') || isAlphanumeric('/') {
		t.Fatal("IsAlphanumeric returned unexpected result")
	}
}
