package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewForms(ctx context.Context, email string) (*forms.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceForms, email); err != nil {
		return nil, fmt.Errorf("forms options: %w", err)
	} else if svc, err := forms.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create forms service: %w", err)
	} else {
		return svc, nil
	}
}
