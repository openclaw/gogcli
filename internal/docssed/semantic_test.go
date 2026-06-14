//nolint:wsl_v5 // Table-driven semantic tests stay compact around assertions.
package docssed

import (
	"strings"
	"testing"
)

func TestEnrichReferences(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		raw   string
		check func(*testing.T, Expression)
	}{
		{
			name: "cell",
			raw:  "s/|1|[A1]:old/new/",
			check: func(t *testing.T, expression Expression) {
				t.Helper()
				if expression.Pattern != "old" || expression.Cell == nil ||
					expression.Cell.TableIndex != 1 || expression.Cell.Row != 1 || expression.Cell.Column != 1 {
					t.Fatalf("expression = %+v, cell = %+v", expression, expression.Cell)
				}
			},
		},
		{
			name: "brace table wildcard",
			raw:  "s/{T=2!1,*}/header/",
			check: func(t *testing.T, expression Expression) {
				t.Helper()
				if expression.Table == nil || expression.Table.TableIndex != 2 ||
					expression.Table.Row != 1 || !expression.Table.RowWild {
					t.Fatalf("table = %+v", expression.Table)
				}
			},
		},
		{
			name: "brace image",
			raw:  "s/{img=logo}//",
			check: func(t *testing.T, expression Expression) {
				t.Helper()
				if expression.Image == nil || !expression.Image.ByAlt || expression.Image.Pattern != "logo" {
					t.Fatalf("image = %+v", expression.Image)
				}
			},
		},
		{
			name: "all tables",
			raw:  "s/|*|//",
			check: func(t *testing.T, expression Expression) {
				t.Helper()
				if expression.Pattern != "" || expression.Table == nil || expression.Table.TableIndex != 0 {
					t.Fatalf("expression = %+v, table = %+v", expression, expression.Table)
				}
			},
		},
		{
			name: "pattern table create",
			raw:  "s/{T=2x3:header}//",
			check: func(t *testing.T, expression Expression) {
				t.Helper()
				if expression.TableCreate == nil || expression.TableCreate.Rows != 2 ||
					expression.TableCreate.Columns != 3 || !expression.TableCreate.Header {
					t.Fatalf("table create = %+v", expression.TableCreate)
				}
			},
		},
		{
			name: "replacement table create",
			raw:  "s/placeholder/{T=2x3}/",
			check: func(t *testing.T, expression Expression) {
				t.Helper()
				if expression.TableCreate == nil || expression.Brace != nil || len(expression.BraceSpans) != 0 {
					t.Fatalf("expression = %+v", expression)
				}
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			program, err := Parse(test.raw)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			program, err = Enrich(program)
			if err != nil {
				t.Fatalf("Enrich: %v", err)
			}
			test.check(t, program.Expressions[0])
		})
	}
}

func TestEnrichBraceReplacement(t *testing.T) {
	t.Parallel()
	program, err := Parse("s/foo/H{,=2}O{b}/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	program, err = Enrich(program)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	expression := program.Expressions[0]
	if expression.Replacement != "H2O" || len(expression.BraceSpans) != 2 {
		t.Fatalf("expression = %+v", expression)
	}
	if expression.Brace == nil || expression.Brace.Bold == nil || !*expression.Brace.Bold {
		t.Fatalf("merged brace = %+v", expression.Brace)
	}
	if expression.BraceSpans[0].Expr.Sub == nil || !*expression.BraceSpans[0].Expr.Sub {
		t.Fatalf("inline brace = %+v", expression.BraceSpans[0])
	}
}

func TestEnrichMalformedBraceReference(t *testing.T) {
	t.Parallel()
	program, err := Parse("s/{T=0}/x/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = Enrich(program)
	if err == nil || !strings.Contains(err.Error(), "brace pattern") {
		t.Fatalf("error = %v", err)
	}
}

func TestEnrichLeavesNonSubstitutionUnchanged(t *testing.T) {
	t.Parallel()
	program, err := Parse("d/foo/")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	enriched, err := Enrich(program)
	if err != nil {
		t.Fatalf("Enrich: %v", err)
	}
	if enriched.Expressions[0].Pattern != "foo" || enriched.Expressions[0].Command != CommandDelete {
		t.Fatalf("expression = %+v", enriched.Expressions[0])
	}
}
