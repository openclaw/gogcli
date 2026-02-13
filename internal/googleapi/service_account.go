package googleapi

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/degree-analytics/ratatosk/internal/config"
)

var newServiceAccountTokenSource = func(ctx context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
	cfg, err := google.JWTConfigFromJSON(keyJSON, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	cfg.Subject = subject

	// Ensure token exchanges don't hang forever.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: defaultHTTPTimeout})

	return cfg.TokenSource(ctx), nil
}

func tokenSourceForServiceAccountScopes(ctx context.Context, email string, scopes []string) (oauth2.TokenSource, string, bool, error) {
	saPath, err := config.ServiceAccountPath(email)
	if err != nil {
		return nil, "", false, fmt.Errorf("service account path: %w", err)
	}

	data, readErr := os.ReadFile(saPath) //nolint:gosec // stored in user config dir
	if readErr == nil {
		ts, tokenErr := newServiceAccountTokenSource(ctx, data, email, scopes)
		if tokenErr != nil {
			return nil, "", false, tokenErr
		}

		return ts, saPath, true, nil
	}

	if !os.IsNotExist(readErr) {
		return nil, "", false, fmt.Errorf("read service account key: %w", readErr)
	}

	// Backwards compatibility: Keep used a dedicated stored service account file.
	keepSAPath, keepErr := config.KeepServiceAccountPath(email)
	if keepErr == nil {
		data, readErr := os.ReadFile(keepSAPath) //nolint:gosec // stored in user config dir
		if readErr == nil {
			ts, tokenErr := newServiceAccountTokenSource(ctx, data, email, scopes)
			if tokenErr != nil {
				return nil, "", false, tokenErr
			}

			return ts, keepSAPath, true, nil
		}

		if !os.IsNotExist(readErr) {
			return nil, "", false, fmt.Errorf("read service account key: %w", readErr)
		}
	}

	legacyPath, legacyErr := config.KeepServiceAccountLegacyPath(email)
	if legacyErr == nil {
		data, readErr := os.ReadFile(legacyPath) //nolint:gosec // stored in user config dir
		if readErr == nil {
			ts, tokenErr := newServiceAccountTokenSource(ctx, data, email, scopes)
			if tokenErr != nil {
				return nil, "", false, tokenErr
			}

			return ts, legacyPath, true, nil
		}

		if !os.IsNotExist(readErr) {
			return nil, "", false, fmt.Errorf("read service account key: %w", readErr)
		}
	}

	return nil, "", false, nil
}
