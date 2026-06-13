package authclient

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/secrets"
)

type (
	contextKey               struct{}
	accessTokenKey           struct{}
	resolverKey              struct{}
	emailReferenceUpdaterKey struct{}
	credentialsReaderKey     struct{}
	secretsStoreOpenerKey    struct{}
)

type (
	ClientResolver        func(email string, override string) (string, error)
	EmailReferenceUpdater func(oldEmail, newEmail string) error
	CredentialsReader     func(client string) (config.ClientCredentials, error)
	SecretsStoreOpener    func() (secrets.Store, error)
)

var (
	errClientResolverRequired        = errors.New("client resolver is required")
	errEmailReferenceUpdaterRequired = errors.New("email reference updater is required")
	errCredentialsReaderRequired     = errors.New("credentials reader is required")
	errSecretsStoreOpenerRequired    = errors.New("secrets store opener is required")
)

func WithClient(ctx context.Context, client string) context.Context {
	client = strings.TrimSpace(client)
	if client == "" {
		return ctx
	}

	return context.WithValue(ctx, contextKey{}, client)
}

func ClientOverrideFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if v := ctx.Value(contextKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}

func WithAccessToken(ctx context.Context, token string) context.Context {
	token = strings.TrimSpace(token)
	if token == "" {
		return ctx
	}

	return context.WithValue(ctx, accessTokenKey{}, token)
}

func AccessTokenFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if v := ctx.Value(accessTokenKey{}); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}

	return ""
}

func WithClientResolver(ctx context.Context, resolver ClientResolver) context.Context {
	if resolver == nil {
		return ctx
	}

	return context.WithValue(ctx, resolverKey{}, resolver)
}

func WithEmailReferenceUpdater(ctx context.Context, updater EmailReferenceUpdater) context.Context {
	if updater == nil {
		return ctx
	}

	return context.WithValue(ctx, emailReferenceUpdaterKey{}, updater)
}

func WithCredentialsReader(ctx context.Context, reader CredentialsReader) context.Context {
	if reader == nil {
		return ctx
	}

	return context.WithValue(ctx, credentialsReaderKey{}, reader)
}

func WithSecretsStoreOpener(ctx context.Context, opener SecretsStoreOpener) context.Context {
	if opener == nil {
		return ctx
	}

	return context.WithValue(ctx, secretsStoreOpenerKey{}, opener)
}

func ReadCredentials(ctx context.Context, client string) (config.ClientCredentials, error) {
	reader := credentialsReaderFromContext(ctx)
	if reader == nil {
		return config.ClientCredentials{}, errCredentialsReaderRequired
	}

	credentials, err := reader(client)
	if err != nil {
		return config.ClientCredentials{}, fmt.Errorf("read credentials: %w", err)
	}

	return credentials, nil
}

func OpenSecretsStore(ctx context.Context) (secrets.Store, error) {
	opener := secretsStoreOpenerFromContext(ctx)
	if opener == nil {
		return nil, errSecretsStoreOpenerRequired
	}

	store, err := opener()
	if err != nil {
		return nil, fmt.Errorf("open secrets store: %w", err)
	}

	return store, nil
}

func UpdateEmailReferences(ctx context.Context, oldEmail, newEmail string) error {
	updater := emailReferenceUpdaterFromContext(ctx)
	if updater == nil {
		return errEmailReferenceUpdaterRequired
	}

	if err := updater(oldEmail, newEmail); err != nil {
		return fmt.Errorf("update email references: %w", err)
	}

	return nil
}

func ResolveClient(ctx context.Context, email string) (string, error) {
	return ResolveClientWithOverride(ctx, email, ClientOverrideFromContext(ctx))
}

func ResolveClientWithOverride(ctx context.Context, email string, override string) (string, error) {
	resolver := clientResolverFromContext(ctx)
	if resolver == nil {
		return "", errClientResolverRequired
	}

	client, err := resolver(email, override)
	if err != nil {
		return "", fmt.Errorf("resolve client: %w", err)
	}

	return client, nil
}

func clientResolverFromContext(ctx context.Context) ClientResolver {
	if ctx == nil {
		return nil
	}

	resolver, _ := ctx.Value(resolverKey{}).(ClientResolver)

	return resolver
}

func emailReferenceUpdaterFromContext(ctx context.Context) EmailReferenceUpdater {
	if ctx == nil {
		return nil
	}

	updater, _ := ctx.Value(emailReferenceUpdaterKey{}).(EmailReferenceUpdater)

	return updater
}

func credentialsReaderFromContext(ctx context.Context) CredentialsReader {
	if ctx == nil {
		return nil
	}

	reader, _ := ctx.Value(credentialsReaderKey{}).(CredentialsReader)

	return reader
}

func secretsStoreOpenerFromContext(ctx context.Context) SecretsStoreOpener {
	if ctx == nil {
		return nil
	}

	opener, _ := ctx.Value(secretsStoreOpenerKey{}).(SecretsStoreOpener)

	return opener
}
