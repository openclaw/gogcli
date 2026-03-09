package googleapi

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/steipete/gogcli/internal/config"
)

func serviceAccountSubject(subject string, serviceAccountEmail string) string {
	if subject == "" || subject == serviceAccountEmail {
		return ""
	}

	return subject
}

var newServiceAccountTokenSource = func(ctx context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
	cfg, err := google.JWTConfigFromJSON(keyJSON, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	// Only set Subject (impersonation) when the caller requests a different
	// identity than the service account itself. When subject matches the
	// SA's client_email we run in pure SA mode: no Domain-Wide Delegation.
	cfg.Subject = serviceAccountSubject(subject, cfg.Email)

	// Ensure token exchanges don't hang forever.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: tokenExchangeTimeout})

	return cfg.TokenSource(ctx), nil
}

func tokenSourceForServiceAccountScopes(ctx context.Context, serviceLabel string, email string, scopes []string) (oauth2.TokenSource, string, bool, error) {
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

	// Keep-specific service account files should only be used for Keep.
	if serviceLabel != "keep" {
		return nil, "", false, nil
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
