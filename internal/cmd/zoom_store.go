package cmd

import (
	"context"
	"os"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/config"
	"github.com/openclaw/gogcli/internal/secrets"
	"github.com/openclaw/gogcli/internal/zoom"
)

func commandZoomStore(ctx context.Context) (*zoom.Store, error) {
	if runtime, ok := app.FromContext(ctx); ok {
		if err := configureRuntimeLayout(runtime, config.PathKindConfig); err != nil {
			return nil, err
		}
		store, err := zoom.NewStore(runtime.Layout, func() (secrets.SecretStore, error) {
			if runtime.Auth.OpenSecretStore != nil {
				return runtime.Auth.OpenSecretStore()
			}
			return openRuntimeSecretsRepository(runtime)
		}, os.LookupEnv)
		if err != nil {
			return nil, err
		}
		return store, nil
	}
	return nil, errRuntimeRequired
}
