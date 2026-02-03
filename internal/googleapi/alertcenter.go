package googleapi

import (
	"context"
	"fmt"

	alertcenter "google.golang.org/api/alertcenter/v1beta1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAlertCenter(ctx context.Context, email string) (*alertcenter.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceAlertCenter, email)
	if err != nil {
		return nil, fmt.Errorf("alertcenter options: %w", err)
	}
	svc, err := alertcenter.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create alertcenter service: %w", err)
	}
	return svc, nil
}
