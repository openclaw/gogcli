package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/accesscontextmanager/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAccessContextManager(ctx context.Context, email string) (*accesscontextmanager.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceAccessContext, email)
	if err != nil {
		return nil, fmt.Errorf("access context options: %w", err)
	}

	svc, err := accesscontextmanager.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create access context service: %w", err)
	}

	return svc, nil
}
