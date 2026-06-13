package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/steipete/gogcli/internal/app"
	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/tracking"
)

var errTrackingSecretStoreRequired = errors.New("tracking secret store is required")

func newTrackingConfigStore(ctx context.Context, secretStore *tracking.SecretStore) (*tracking.ConfigStore, error) {
	layout, err := commandLayout(ctx, config.PathKindConfig, config.PathKindState)
	if err != nil {
		return nil, err
	}
	legacyConfigBase := ""
	if !layout.ExplicitState {
		legacyConfigBase, err = config.ResolveUserConfigBase()
		if err != nil {
			return nil, err
		}
	}
	return tracking.NewConfigStore(layout, legacyConfigBase, secretStore)
}

func newTrackingSecretStore(ctx context.Context) (*tracking.SecretStore, error) {
	runtime, ok := app.FromContext(ctx)
	if !ok || runtime.Auth.OpenSecretStore == nil {
		return nil, errTrackingSecretStoreRequired
	}

	store, err := runtime.Auth.OpenSecretStore()
	if err != nil {
		return nil, fmt.Errorf("open tracking secret store: %w", err)
	}

	secretStore, err := tracking.NewSecretStore(store)
	if err != nil {
		return nil, fmt.Errorf("create tracking secret store: %w", err)
	}

	return secretStore, nil
}

func loadTrackingConfigForAccount(ctx context.Context, flags *RootFlags) (string, *tracking.Config, *tracking.ConfigStore, *tracking.SecretStore, error) {
	return loadTrackingConfigForAccountWith(ctx, flags, true)
}

func loadTrackingConfigMetadataForAccount(ctx context.Context, flags *RootFlags) (string, *tracking.Config, *tracking.ConfigStore, *tracking.SecretStore, error) {
	return loadTrackingConfigForAccountWith(ctx, flags, false)
}

func loadTrackingConfigForAccountWith(
	ctx context.Context,
	flags *RootFlags,
	hydrate bool,
) (string, *tracking.Config, *tracking.ConfigStore, *tracking.SecretStore, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return "", nil, nil, nil, err
	}

	cfg, configStore, secretStore, err := loadTrackingConfig(ctx, account, hydrate)
	if err != nil {
		return "", nil, nil, nil, err
	}

	return account, cfg, configStore, secretStore, nil
}

func loadTrackingConfig(
	ctx context.Context,
	account string,
	hydrate bool,
) (*tracking.Config, *tracking.ConfigStore, *tracking.SecretStore, error) {
	configStore, err := newTrackingConfigStore(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err := configStore.LoadMetadata(account)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load tracking config: %w", err)
	}
	if !hydrate || !cfg.NeedsSecretStore() {
		return cfg, configStore, nil, nil
	}

	secretStore, err := newTrackingSecretStore(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	configStore, err = newTrackingConfigStore(ctx, secretStore)
	if err != nil {
		return nil, nil, nil, err
	}

	cfg, err = configStore.Load(account)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load tracking config: %w", err)
	}

	return cfg, configStore, secretStore, nil
}

func ensureTrackingSecretStore(ctx context.Context, secretStore *tracking.SecretStore) (*tracking.SecretStore, error) {
	if secretStore != nil {
		return secretStore, nil
	}

	return newTrackingSecretStore(ctx)
}
