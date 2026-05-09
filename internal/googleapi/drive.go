package googleapi

import (
	"context"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewDrive(ctx context.Context, email string) (*drive.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceDrive, "drive", drive.NewService)
}
