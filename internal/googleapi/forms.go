package googleapi

import (
	"context"

	"google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewForms(ctx context.Context, email string) (*forms.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceForms, "forms", forms.NewService)
}
