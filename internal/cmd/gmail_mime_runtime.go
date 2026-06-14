package cmd

import (
	"context"
	"io"
	"time"
)

func mailDateLocation(ctx context.Context, diagnostics io.Writer) (*time.Location, error) {
	loc, err := getConfiguredTimezone(ctx, "", diagnostics)
	if err != nil {
		return nil, err
	}
	if loc != nil {
		return loc, nil
	}
	return time.Local, nil
}
