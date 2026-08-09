package googleapi

import (
	"context"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewSitesDrive(ctx context.Context, email string) (*drive.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceSites, "sites", drive.NewService)
}
