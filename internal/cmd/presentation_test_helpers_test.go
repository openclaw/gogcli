package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/openclaw/gogcli/internal/outfmt"
)

func renderPlainTable[T any](t *testing.T, rows []T, columns []outfmt.Column[T]) string {
	t.Helper()

	var output bytes.Buffer
	ctx := outfmt.WithMode(context.Background(), outfmt.Mode{Plain: true})
	if err := outfmt.WriteTable(ctx, &output, rows, columns); err != nil {
		t.Fatalf("WriteTable: %v", err)
	}
	return output.String()
}

func assertTableOutput(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
