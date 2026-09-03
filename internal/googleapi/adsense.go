package googleapi

import (
	"context"

	adsenseapi "google.golang.org/api/adsense/v2"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewAdSense(ctx context.Context, email string) (*adsenseapi.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceAdSense, "adsense", adsenseapi.NewService)
}
