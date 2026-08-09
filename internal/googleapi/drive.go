package googleapi

import (
	"context"

	drivev2 "google.golang.org/api/drive/v2"
	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewDrive(ctx context.Context, email string) (*drive.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceDrive, "drive", drive.NewService)
}

func NewDriveV2(ctx context.Context, email string) (*drivev2.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceDrive, "drive", drivev2.NewService)
}
