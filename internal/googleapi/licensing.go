package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/licensing/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewLicensing(ctx context.Context, email string) (*licensing.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceLicensing, email)
	if err != nil {
		return nil, fmt.Errorf("licensing options: %w", err)
	}

	svc, err := licensing.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create licensing service: %w", err)
	}

	return svc, nil
}
