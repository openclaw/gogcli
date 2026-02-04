package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/iam/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewIAM(ctx context.Context, email string) (*iam.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceIAM, email)
	if err != nil {
		return nil, fmt.Errorf("iam options: %w", err)
	}

	svc, err := iam.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create iam service: %w", err)
	}

	return svc, nil
}
