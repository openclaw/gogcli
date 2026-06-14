package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/gmailwatch"
)

type (
	gmailWatchStore = gmailwatch.Repository
	gmailWatchState = gmailwatch.State
	gmailWatchHook  = gmailwatch.Hook
)

func gmailWatchStatePath(layout config.Layout, account string) (string, error) {
	dir := layout.GmailWatchDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("ensure gmail watch dir: %w", err)
	}

	name := sanitizeAccountForPath(account)
	path := filepath.Join(dir, name+".json")
	if _, statErr := os.Stat(path); statErr == nil {
		return path, nil
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return "", statErr
	}

	if !layout.ExplicitState {
		legacyDir := layout.LegacyGmailWatchDir()
		legacyPath := filepath.Join(legacyDir, name+".json")
		if legacyPath != path {
			if _, statErr := os.Stat(legacyPath); statErr == nil {
				return legacyPath, nil
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return "", statErr
			}
		}
	}

	return path, nil
}

func sanitizeAccountForPath(account string) string {
	clean := strings.TrimSpace(strings.ToLower(account))
	if clean == "" {
		return "unknown"
	}

	var builder strings.Builder
	builder.Grow(len(clean))
	for _, char := range clean {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.' || char == '-' || char == '_' || char == '@':
			builder.WriteRune('_')
		case char > unicode.MaxASCII:
			builder.WriteRune('_')
		default:
			builder.WriteRune('_')
		}
	}

	return builder.String()
}

func newGmailWatchStore(ctx context.Context, account string) (*gmailWatchStore, error) {
	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindState)
	if err != nil {
		return nil, err
	}

	return newGmailWatchStoreForLayout(layout, account)
}

func newGmailWatchStoreForLayout(layout config.Layout, account string) (*gmailWatchStore, error) {
	path, err := gmailWatchStatePath(layout, account)
	if err != nil {
		return nil, err
	}

	return gmailwatch.New(path, gmailwatch.Options{}), nil
}

func loadGmailWatchStore(ctx context.Context, account string) (*gmailWatchStore, error) {
	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindState)
	if err != nil {
		return nil, err
	}

	return loadGmailWatchStoreForLayout(layout, account)
}

func loadGmailWatchStoreForLayout(layout config.Layout, account string) (*gmailWatchStore, error) {
	path, err := gmailWatchStatePath(layout, account)
	if err != nil {
		return nil, err
	}

	return gmailwatch.Load(path, gmailwatch.Options{})
}

func readGmailWatchStateOptional(ctx context.Context, account string) (gmailWatchState, bool, error) {
	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindState)
	if err != nil {
		return gmailWatchState{}, false, err
	}

	return readGmailWatchStateOptionalForLayout(layout, account)
}

func readGmailWatchState(ctx context.Context, account string) (gmailWatchState, error) {
	state, found, err := readGmailWatchStateOptional(ctx, account)
	if err != nil {
		return gmailWatchState{}, err
	}
	if !found {
		return gmailWatchState{}, gmailwatch.ErrNotFound
	}

	return state, nil
}

func readGmailWatchStateOptionalForLayout(layout config.Layout, account string) (gmailWatchState, bool, error) {
	name := sanitizeAccountForPath(account) + ".json"
	paths := []string{filepath.Join(layout.GmailWatchDir(), name)}
	if !layout.ExplicitState {
		legacyPath := filepath.Join(layout.LegacyGmailWatchDir(), name)
		if legacyPath != paths[0] {
			paths = append(paths, legacyPath)
		}
	}

	return gmailwatch.ReadOptional(paths...)
}

func isStaleHistoryID(currentRaw, candidateRaw string) (bool, error) {
	return gmailwatch.IsStaleHistoryID(currentRaw, candidateRaw)
}

func parseHistoryID(raw string) (uint64, error) {
	return gmailwatch.ParseHistoryID(raw)
}

func formatHistoryID(id uint64) string {
	return gmailwatch.FormatHistoryID(id)
}
