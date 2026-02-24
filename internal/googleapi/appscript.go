package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/script/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAppScript(ctx context.Context, email string) (*script.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAppScript, email); err != nil {
		return nil, fmt.Errorf("appscript options: %w", err)
	} else if svc, err := script.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create appscript service: %w", err)
	} else {
		return svc, nil
	}
}
