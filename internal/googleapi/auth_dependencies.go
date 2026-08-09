package googleapi

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/openclaw/gogcli/internal/authclient"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/googleauth"
	"github.com/openclaw/gogcli/internal/secrets"
)

type AuthMode string

const (
	AuthModeStored AuthMode = ""
	AuthModeADC    AuthMode = "adc"
)

type (
	ServiceAccountStoreResolver   func() (*config.ServiceAccountStore, error)
	ADCTokenSourceFunc            func(context.Context, ...string) (oauth2.TokenSource, error)
	ServiceAccountTokenSourceFunc func(context.Context, []byte, string, []string) (oauth2.TokenSource, error)
)

type AuthDependencies struct {
	ResolveClient             authclient.ClientResolver
	ReadCredentials           authclient.CredentialsReader
	OpenTokens                authclient.SecretsStoreOpener
	ServiceAccounts           ServiceAccountStoreResolver
	UpdateEmailReferences     googleauth.EmailReferenceUpdater
	Mode                      AuthMode
	ADCTokenSource            ADCTokenSourceFunc
	ServiceAccountTokenSource ServiceAccountTokenSourceFunc
}

var (
	errAuthDependenciesRequired          = errors.New("google API auth dependencies are required")
	errAuthClientResolverRequired        = errors.New("google API auth client resolver is required")
	errAuthCredentialsReaderRequired     = errors.New("google API auth credentials reader is required")
	errAuthTokenStoreOpenerRequired      = errors.New("google API auth token store opener is required")
	errServiceAccountStoreRequired       = errors.New("service account store resolver is required")
	errEmailReferenceUpdaterRequired     = errors.New("google API auth email reference updater is required")
	errADCTokenSourceRequired            = errors.New("ADC token source factory is required")
	errServiceAccountTokenSourceRequired = errors.New("service account token source factory is required")
)

type authDependenciesContextKey struct{}

func ParseAuthMode(value string) AuthMode {
	if value == string(AuthModeADC) {
		return AuthModeADC
	}

	return AuthModeStored
}

func WithAuthDependencies(ctx context.Context, dependencies AuthDependencies) context.Context {
	return context.WithValue(ctx, authDependenciesContextKey{}, dependencies)
}

func authDependenciesFromContext(ctx context.Context) (AuthDependencies, bool) {
	if ctx == nil {
		return AuthDependencies{}, false
	}

	dependencies, ok := ctx.Value(authDependenciesContextKey{}).(AuthDependencies)

	return dependencies, ok
}

func requireAuthDependencies(ctx context.Context) (AuthDependencies, error) {
	dependencies, ok := authDependenciesFromContext(ctx)
	if !ok {
		return AuthDependencies{}, errAuthDependenciesRequired
	}

	return dependencies, nil
}

func (d AuthDependencies) resolveClient(email, override string) (string, error) {
	if d.ResolveClient == nil {
		return "", errAuthClientResolverRequired
	}

	client, err := d.ResolveClient(email, override)
	if err != nil {
		return "", fmt.Errorf("resolve client: %w", err)
	}

	return client, nil
}

func (d AuthDependencies) readCredentials(client string) (config.ClientCredentials, error) {
	if d.ReadCredentials == nil {
		return config.ClientCredentials{}, errAuthCredentialsReaderRequired
	}

	credentials, err := d.ReadCredentials(client)
	if err != nil {
		return config.ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
	}

	return credentials, nil
}

func (d AuthDependencies) openTokens() (secrets.Store, error) {
	if d.OpenTokens == nil {
		return nil, errAuthTokenStoreOpenerRequired
	}

	store, err := d.OpenTokens()
	if err != nil {
		return nil, fmt.Errorf("open token store: %w", err)
	}

	if store == nil {
		return nil, errAuthTokenStoreOpenerRequired
	}

	return store, nil
}

func (d AuthDependencies) serviceAccountStore() (*config.ServiceAccountStore, error) {
	if d.ServiceAccounts == nil {
		return nil, errServiceAccountStoreRequired
	}

	store, err := d.ServiceAccounts()
	if err != nil {
		return nil, fmt.Errorf("resolve service account store: %w", err)
	}

	if store == nil {
		return nil, errServiceAccountStoreRequired
	}

	return store, nil
}

func (d AuthDependencies) updateEmailReferences(oldEmail, newEmail string) error {
	if d.UpdateEmailReferences == nil {
		return errEmailReferenceUpdaterRequired
	}

	if err := d.UpdateEmailReferences(oldEmail, newEmail); err != nil {
		return fmt.Errorf("update email references: %w", err)
	}

	return nil
}

func (d AuthDependencies) adcTokenSource(ctx context.Context, scopes []string) (oauth2.TokenSource, error) {
	if d.ADCTokenSource == nil {
		return nil, errADCTokenSourceRequired
	}

	tokenSource, err := d.ADCTokenSource(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("ADC token source: %w", err)
	}

	return tokenSource, nil
}

func (d AuthDependencies) serviceAccountTokenSource(ctx context.Context, keyJSON []byte, subject string, scopes []string) (oauth2.TokenSource, error) {
	if d.ServiceAccountTokenSource == nil {
		return nil, errServiceAccountTokenSourceRequired
	}

	return d.ServiceAccountTokenSource(ctx, keyJSON, subject, scopes)
}

func DefaultADCTokenSource(ctx context.Context, scopes ...string) (oauth2.TokenSource, error) {
	tokenSource, err := google.DefaultTokenSource(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("default token source: %w", err)
	}

	return tokenSource, nil
}
