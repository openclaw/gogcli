package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/docs/v1"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func NewDocs(ctx context.Context, email string) (*docs.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceDocs, email); err != nil {
		return nil, fmt.Errorf("docs options: %w", err)
	} else if svc, err := docs.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create docs service: %w", err)
	} else {
		return svc, nil
	}
}
