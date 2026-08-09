package googleapi

import (
	"context"
	"fmt"

	analyticsadmin "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdata "google.golang.org/api/analyticsdata/v1beta"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewAnalyticsAdmin(ctx context.Context, email string) (*analyticsadmin.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAnalytics, email); err != nil {
		return nil, fmt.Errorf("analyticsadmin options: %w", err)
	} else if svc, err := analyticsadmin.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create analyticsadmin service: %w", err)
	} else {
		return svc, nil
	}
}

func NewAnalyticsData(ctx context.Context, email string) (*analyticsdata.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceAnalytics, email); err != nil {
		return nil, fmt.Errorf("analyticsdata options: %w", err)
	} else if svc, err := analyticsdata.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create analyticsdata service: %w", err)
	} else {
		return svc, nil
	}
}
