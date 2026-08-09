package googleapi

import (
	"context"
	"fmt"

	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewSearchConsole(ctx context.Context, email string) (*searchconsoleapi.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceSearchConsole, email); err != nil {
		return nil, fmt.Errorf("searchconsole options: %w", err)
	} else if svc, err := searchconsoleapi.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create searchconsole service: %w", err)
	} else {
		return svc, nil
	}
}
