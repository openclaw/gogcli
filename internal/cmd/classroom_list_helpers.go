package cmd

import (
	"context"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func fetchClassroomPagedList[T any](all bool, page string, fetch func(string) ([]*T, string, error)) ([]*T, string, error) {
	return loadPagedItems(page, all, fetch)
}

func nonNilClassroomItems[T any](items []*T) []*T {
	if items == nil {
		return []*T{}
	}
	return items
}

func compactClassroomRows[T any](items []*T) []*T {
	rows := make([]*T, 0, len(items))
	for _, item := range items {
		if item != nil {
			rows = append(rows, item)
		}
	}
	return rows
}

func writeClassroomPagedList[T any](
	ctx context.Context,
	jsonKey string,
	items []*T,
	nextPageToken string,
	emptyMessage string,
	failEmpty bool,
	hintOnEmpty bool,
	columns []outfmt.Column[*T],
) error {
	items = nonNilClassroomItems(items)
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
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
			printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
		}
		return failEmptyExit(failEmpty)
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), compactClassroomRows(items), columns); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}
