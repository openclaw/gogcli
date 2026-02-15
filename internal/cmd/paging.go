package cmd

import (
	"fmt"
	"strings"
)

const emptyResultsExitCode = 3

func failEmptyExit(failEmpty bool) error {
	if !failEmpty {
		return nil
	}
	return &ExitError{Code: emptyResultsExitCode, Err: nil}
}

// collectAllPages keeps calling fetch until it returns an empty next page token.
// It guards against pagination loops by tracking seen page tokens.
func collectAllPages[T any](startPageToken string, fetch func(pageToken string) ([]T, string, error)) ([]T, error) {
	pageToken := strings.TrimSpace(startPageToken)
	seen := map[string]bool{}

	var out []T
	for i := 0; i < 10_000; i++ {
		if seen[pageToken] {
			return nil, fmt.Errorf("pagination loop: repeated page token %q", pageToken)
		}
		seen[pageToken] = true

		items, next, err := fetch(pageToken)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)

		next = strings.TrimSpace(next)
		if next == "" {
			return out, nil
		}
		pageToken = next
	}
	return nil, fmt.Errorf("pagination exceeded max pages")
}
