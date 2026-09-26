package googleapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func serviceAccountSubject(subject string, serviceAccountEmail string) string {
	subject = strings.TrimSpace(subject)
	serviceAccountEmail = strings.TrimSpace(serviceAccountEmail)

	if subject == "" || strings.EqualFold(subject, serviceAccountEmail) {
		return ""
	}

	return subject
}

func DefaultServiceAccountTokenSource(ctx context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
	httpClient := &http.Client{Timeout: tokenExchangeTimeout}

	if sa, ok, err := ParseSignJWTServiceAccount(keyJSON); ok {
		if err != nil {
			return nil, err
		}

		sign, err := adcSignJWTSigner(ctx)
		if err != nil {
			return nil, err
		}

		return newSignJWTTokenSource(ctx, sa, subject, scopes, sign, httpClient), nil
	}

	cfg, err := google.JWTConfigFromJSON(keyJSON, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse service account: %w", err)
	}
	// Only set Subject (impersonation) when the caller requests a different
	// identity than the service account itself. When subject matches the
	// SA's client_email we run in pure SA mode: no Domain-Wide Delegation.
	cfg.Subject = serviceAccountSubject(subject, cfg.Email)

	// Ensure token exchanges don't hang forever.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httpClient)

	return cfg.TokenSource(ctx), nil
}

func tokenSourceForServiceAccountScopes(
	ctx context.Context,
	dependencies AuthDependencies,
	serviceLabel string,
	email string,
	scopes []string,
) (oauth2.TokenSource, string, bool, error) {
	store, err := dependencies.serviceAccountStore()
	if err != nil {
		return nil, "", false, err
	}

	file, exists, err := store.Read(email, serviceLabel == "keep")
	if err != nil {
		return nil, "", false, fmt.Errorf("read service account: %w", err)
	}

	if !exists {
		return nil, "", false, nil
	}

	ts, err := dependencies.serviceAccountTokenSource(ctx, file.Data, email, scopes)
	if err != nil {
		return nil, "", false, err
	}

	return ts, file.Path, true, nil
}
