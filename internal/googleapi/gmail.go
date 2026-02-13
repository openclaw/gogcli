package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/gmail/v1"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func NewGmail(ctx context.Context, email string) (*gmail.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceGmail, email); err != nil {
		return nil, fmt.Errorf("gmail options: %w", err)
	} else if svc, err := gmail.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create gmail service: %w", err)
	} else {
		return svc, nil
	}
}
