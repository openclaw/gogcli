package cmd

import (
	"context"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
)

type pageFetchFunc[T any] func(pageToken string) ([]T, string, error)

func loadPagedItems[T any](page string, all bool, fetch pageFetchFunc[T]) ([]T, string, error) {
	if all {
		items, err := collectAllPages(page, fetch)
		if err != nil {
			var zero []T
			return zero, "", err
		}
		return items, "", nil
	}
	return fetch(page)
}

func writePagedJSONResult(ctx context.Context, payload map[string]any, emptyCount int, failEmpty bool) error {
	if err := outfmt.WriteJSON(ctx, os.Stdout, payload); err != nil {
		return err
	}
	if emptyCount == 0 {
		return failEmptyExit(failEmpty)
	}
	return nil
}
