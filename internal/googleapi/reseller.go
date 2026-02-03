package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/reseller/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewReseller(ctx context.Context, email string) (*reseller.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceReseller, email)
	if err != nil {
		return nil, fmt.Errorf("reseller options: %w", err)
	}
	svc, err := reseller.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create reseller service: %w", err)
	}
	return svc, nil
}
