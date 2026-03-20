package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/meet/v2"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewMeet(ctx context.Context, email string) (*meet.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceMeet, email); err != nil {
		return nil, fmt.Errorf("meet options: %w", err)
	} else if svc, err := meet.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create meet service: %w", err)
	} else {
		return svc, nil
	}
}
