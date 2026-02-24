package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewSlides(ctx context.Context, email string) (*slides.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceSlides, email); err != nil {
		return nil, fmt.Errorf("slides options: %w", err)
	} else if svc, err := slides.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create slides service: %w", err)
	} else {
		return svc, nil
	}
}
