package cmd

import "testing"

func TestParseTableCellRef(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantNil    bool
		wantTable  int
		wantRow    int
		wantCol    int
		wantSubPat string
	}{
		{"numeric basic", "|1|[2,3]", false, 1, 2, 3, ""},
		{"excel style", "|1|[A1]", false, 1, 1, 1, ""},
		{"negative table", "|-1|[1,1]", false, -1, 1, 1, ""},
		{"with subpattern", "|1|[2,4]:old", false, 1, 2, 4, "old"},
		{"excel B2", "|2|[B2]", false, 2, 2, 2, ""},
		{"not a ref", "hello", true, 0, 0, 0, ""},
		{"no bracket", "|1|foo", true, 0, 0, 0, ""},
		{"single pipe", "|1[2,3]", true, 0, 0, 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := parseTableCellRef(tt.input)
			if tt.wantNil {
				if ref != nil {
					t.Errorf("expected nil, got %+v", ref)
				}
				return
			}
			if ref == nil {
				t.Fatal("expected non-nil ref")
				return
			}
			if ref.tableIndex != tt.wantTable {
				t.Errorf("tableIndex = %d, want %d", ref.tableIndex, tt.wantTable)
			}
			if ref.row != tt.wantRow {
				t.Errorf("row = %d, want %d", ref.row, tt.wantRow)
			}
			if ref.col != tt.wantCol {
				t.Errorf("col = %d, want %d", ref.col, tt.wantCol)
			}
			if ref.subPattern != tt.wantSubPat {
				t.Errorf("subPattern = %q, want %q", ref.subPattern, tt.wantSubPat)
			}
		})
	}
}

func TestParseTableCellRefExcel(t *testing.T) {
	tests := []struct {
		input   string
		wantRow int
		wantCol int
	}{
		{"A1", 1, 1},
		{"B2", 2, 2},
		{"C10", 10, 3},
		{"Z1", 1, 26},
		{"AA1", 1, 27},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			row, col, ok := parseExcelRef(tt.input)
			if !ok {
				t.Fatal("parseExcelRef returned false")
			}
			if row != tt.wantRow {
				t.Errorf("row = %d, want %d", row, tt.wantRow)
			}
			if col != tt.wantCol {
				t.Errorf("col = %d, want %d", col, tt.wantCol)
			}
		})
	}
}
