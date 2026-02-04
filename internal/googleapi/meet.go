package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewMeet(ctx context.Context, email string) (*meet.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceMeet, email)
	if err != nil {
		return nil, fmt.Errorf("meet options: %w", err)
	}

	svc, err := meet.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create meet service: %w", err)
	}

	return svc, nil
}
