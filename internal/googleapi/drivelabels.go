package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/drivelabels/v2"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewDriveLabels(ctx context.Context, email string) (*drivelabels.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceDriveLabels, email)
	if err != nil {
		return nil, fmt.Errorf("drive labels options: %w", err)
	}
	svc, err := drivelabels.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create drive labels service: %w", err)
	}
	return svc, nil
}
