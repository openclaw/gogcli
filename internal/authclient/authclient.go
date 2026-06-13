package authclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type (
	contextKey               struct{}
	accessTokenKey           struct{}
	resolverKey              struct{}
	emailReferenceUpdaterKey struct{}
)

type (
	ClientResolver        func(email string, override string) (string, error)
	EmailReferenceUpdater func(oldEmail, newEmail string) error
)

var (
	errClientResolverRequired        = errors.New("client resolver is required")
	errEmailReferenceUpdaterRequired = errors.New("email reference updater is required")
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
