package cmd

import (
	"context"
	"errors"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
)

func requireRawResponse[T any](response *T, notFoundMessage string) (*T, error) {
	if response == nil {
		return nil, errors.New(notFoundMessage)
	}
	return response, nil
}

func writeRawJSON(ctx context.Context, value any, pretty bool) error {
	return outfmt.WriteRaw(ctx, os.Stdout, value, outfmt.RawOptions{Pretty: pretty})
}
