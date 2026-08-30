package googleapi

import (
	"context"
	"fmt"

	adsenseapi "google.golang.org/api/adsense/v2"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewAdSense(ctx context.Context, email string) (*adsenseapi.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAdSense, email); err != nil {
		return nil, fmt.Errorf("adsense options: %w", err)
	} else if svc, err := adsenseapi.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create adsense service: %w", err)
	} else {
		return svc, nil
	}
}
