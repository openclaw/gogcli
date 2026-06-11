package cmd

import (
	"context"
	"os"
	"sort"
	"strconv"

	"github.com/steipete/gogcli/internal/outfmt"
)

type ExitCodesCmd struct{}

func (c *ExitCodesCmd) Run(ctx context.Context) error {
	// Always emit untransformed JSON, even if the caller enabled global JSON transforms.
	ctx = outfmt.WithJSONTransform(ctx, outfmt.JSONTransform{})

	codes := map[string]int{
		"ok":                0,
		"error":             1,
		"usage":             2,
		"empty_results":     emptyResultsExitCode,
		"auth_required":     exitCodeAuthRequired,
		"not_found":         exitCodeNotFound,
		"orphaned":          exitCodeOrphaned,
		"permission_denied": exitCodePermissionDenied,
		"rate_limited":      exitCodeRateLimited,
		"retryable":         exitCodeRetryable,
		"config":            exitCodeConfig,
		"cancelled":         exitCodeCancelled,
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"exit_codes": codes})
	}

	if outfmt.IsPlain(ctx) {
		keys := make([]string, 0, len(codes))
		for k := range codes {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			_, _ = os.Stdout.WriteString(k + "\t" + strconv.Itoa(codes[k]) + "\n")
		}
		return nil
	}

	keys := make([]string, 0, len(codes))
	for k := range codes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = os.Stdout.WriteString(k + ": " + strconv.Itoa(codes[k]) + "\n")
	}
	return nil
}
