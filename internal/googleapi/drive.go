package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/drive/v3"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func NewDrive(ctx context.Context, email string) (*drive.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceDrive, email); err != nil {
		return nil, fmt.Errorf("drive options: %w", err)
	} else if svc, err := drive.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	} else {
		return svc, nil
	}
}
