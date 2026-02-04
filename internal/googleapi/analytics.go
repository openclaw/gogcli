package googleapi

import (
	"context"
	"fmt"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAnalyticsAdmin(ctx context.Context, email string) (*analyticsadmin.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceAnalytics, email)
	if err != nil {
		return nil, fmt.Errorf("analytics admin options: %w", err)
	}
	svc, err := analyticsadmin.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create analytics admin service: %w", err)
	}
	return svc, nil
}
