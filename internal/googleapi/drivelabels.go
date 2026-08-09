package googleapi

import (
	"context"

	"google.golang.org/api/drivelabels/v2"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewDriveLabels(ctx context.Context, email string) (*drivelabels.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceDriveLabels, "drive labels", drivelabels.NewService)
}
