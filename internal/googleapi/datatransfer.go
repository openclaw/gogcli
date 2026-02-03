package googleapi

import (
	"context"
	"fmt"

	datatransfer "google.golang.org/api/admin/datatransfer/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewDataTransfer(ctx context.Context, email string) (*datatransfer.Service, error) {
	opts, err := optionsForAccount(ctx, googleauth.ServiceDataTransfer, email)
	if err != nil {
		return nil, fmt.Errorf("datatransfer options: %w", err)
	}
	svc, err := datatransfer.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create datatransfer service: %w", err)
	}
	return svc, nil
}
