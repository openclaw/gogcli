package googleapi

import (
	"context"
	"fmt"

	reports "google.golang.org/api/admin/reports/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewReports(ctx context.Context, email string) (*reports.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceReports, email)
	if err != nil {
		return nil, fmt.Errorf("reports options: %w", err)
	}

	svc, err := reports.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create reports service: %w", err)
	}

	return svc, nil
}
