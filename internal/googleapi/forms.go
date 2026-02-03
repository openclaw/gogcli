package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewForms(ctx context.Context, email string) (*forms.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceForms, email)
	if err != nil {
		return nil, fmt.Errorf("forms options: %w", err)
	}
	svc, err := forms.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create forms service: %w", err)
	}
	return svc, nil
}
