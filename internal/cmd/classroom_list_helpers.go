package cmd

import (
	"context"
	"io"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func fetchClassroomPagedList[T any](all bool, page string, fetch func(string) ([]*T, string, error)) ([]*T, string, error) {
	if all {
		items, err := collectAllPages(page, fetch)
		if err != nil {
			return nil, "", err
		}
		return items, "", nil
	}
	return fetch(page)
}

func writeClassroomPagedList[T any](
	ctx context.Context,
	jsonKey string,
	items []*T,
	nextPageToken string,
	emptyMessage string,
	failEmpty bool,
	hintOnEmpty bool,
	printTable func(io.Writer),
) error {
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			jsonKey:         items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}

	u := ui.FromContext(ctx)
	if len(items) == 0 {
		u.Err().Println(emptyMessage)
		if hintOnEmpty {
			printNextPageHint(u, nextPageToken)
		}
		return failEmptyExit(failEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	printTable(w)
	printNextPageHint(u, nextPageToken)
	return nil
}
