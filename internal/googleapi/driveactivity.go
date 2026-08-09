package googleapi

import (
	"context"

	"google.golang.org/api/driveactivity/v2"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewDriveActivity(ctx context.Context, email string) (*driveactivity.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceDriveActivity, "drive activity", driveactivity.NewService)
}
