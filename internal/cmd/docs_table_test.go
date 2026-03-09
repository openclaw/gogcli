package cmd

import (
	"math"
	"testing"
)

func TestParseTableRef(t *testing.T) {
	tests := []struct {
		input string
		want  int
		ok    bool
		desc  string
	}{
		{"|1|", 1, true, "first table"},
		{"|2|", 2, true, "second table"},
		{"|-1|", -1, true, "last table"},
		{"|-2|", -2, true, "second to last"},
		{"|*|", math.MinInt32, true, "all tables"},
		{"|0|", 0, false, "zero invalid"},
		{"|abc|", 0, false, "non-numeric"},
		{"|3x4|", 0, false, "table create spec"},
		{"|3X4|", 0, false, "table create uppercase"},
		{"hello", 0, false, "plain text"},
		{"|1|[A1]", 0, false, "cell ref"},
		{"||", 0, false, "empty"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got, ok := parseTableRef(tt.input)
			if ok != tt.ok {
				t.Errorf("parseTableRef(%q) ok=%v, want ok=%v", tt.input, ok, tt.ok)
				return
			}
			if ok && got != tt.want {
				t.Errorf("parseTableRef(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseTableCreate(t *testing.T) {
	tests := []struct {
		input string
		want  *tableCreateSpec
		desc  string
	}{
		{"|3x4|", &tableCreateSpec{rows: 3, cols: 4, header: false}, "basic 3x4"},
		{"|3x4:header|", &tableCreateSpec{rows: 3, cols: 4, header: true}, "3x4 with header"},
		{"|1x1|", &tableCreateSpec{rows: 1, cols: 1, header: false}, "minimum 1x1"},
		{"|10x26|", &tableCreateSpec{rows: 10, cols: 26, header: false}, "max columns"},
		{"|100x1|", &tableCreateSpec{rows: 100, cols: 1, header: false}, "max rows"},
		{" |3x4| ", &tableCreateSpec{rows: 3, cols: 4, header: false}, "with whitespace"},
		{"|3X4|", &tableCreateSpec{rows: 3, cols: 4, header: false}, "uppercase X"},
		{"|3x4:HEADER|", &tableCreateSpec{rows: 3, cols: 4, header: true}, "uppercase HEADER"},

		// Invalid cases
		{"hello", nil, "plain text"},
		{"|3x|", nil, "missing cols"},
		{"|x4|", nil, "missing rows"},
		{"|0x4|", nil, "zero rows"},
		{"|3x0|", nil, "zero cols"},
		{"|101x1|", nil, "too many rows"},
		{"|1x27|", nil, "too many cols"},
		{"|3x4:foo|", nil, "bad suffix"},
		{"|-1x4|", nil, "negative rows"},
		{"|3x-1|", nil, "negative cols"},
		{"||", nil, "empty"},
		{"|abc|", nil, "no x separator"},
		{"|1|[A1]", nil, "cell ref not table create"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := parseTableCreate(tt.input)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseTableCreate(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseTableCreate(%q) = nil, want %+v", tt.input, tt.want)
				return
			}
			if got.rows != tt.want.rows || got.cols != tt.want.cols || got.header != tt.want.header {
				t.Errorf("parseTableCreate(%q) = {rows:%d, cols:%d, header:%v}, want {rows:%d, cols:%d, header:%v}",
					tt.input, got.rows, got.cols, got.header, tt.want.rows, tt.want.cols, tt.want.header)
			}
		})
	}
}
