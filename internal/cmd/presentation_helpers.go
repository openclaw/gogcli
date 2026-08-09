package cmd

import "github.com/openclaw/gogcli/internal/outfmt"

func sanitizedColumn[T any](header string, value func(T) string) outfmt.Column[T] {
	return outfmt.Column[T]{
		Header: header,
		Value:  func(row T) string { return sanitizeTab(value(row)) },
	}
}
