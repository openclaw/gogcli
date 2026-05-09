package outfmt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// RawOptions configures WriteRaw.
type RawOptions struct {
	// Pretty emits indented JSON (2-space). Default is compact single-line.
	Pretty bool
}

// WriteRaw marshals v as JSON and writes it to w, emitting the value bare
// (no envelope/wrapper). Intended for `gog <group> raw` subcommands that
// expose the canonical Google API response for programmatic consumption.
//
// Compact by default; pass RawOptions{Pretty: true} for indented output.
// Always appends a trailing newline for pipe friendliness.
// HTML escaping is disabled so URLs with & survive unchanged.
func WriteRaw(_ context.Context, w io.Writer, v any, opts RawOptions) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	if opts.Pretty {
		enc.SetIndent("", "  ")
	}

	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode raw json: %w", err)
	}

	return nil
}
