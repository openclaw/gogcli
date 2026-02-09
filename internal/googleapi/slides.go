package googleapi

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewSlides(ctx context.Context, email string) (*slides.Service, error) {
	slog.Debug("creating slides service", "email", email)

	opts, err := optionsForAccount(ctx, googleauth.ServiceSlides, email)
	if err != nil {
		return nil, fmt.Errorf("slides options: %w", err)
	}

	svc, err := slides.NewService(ctx, opts...)
	if err != nil {
		slog.Error("failed to create slides service", "email", email, "error", err)
		return nil, fmt.Errorf("create slides service: %w", err)
	}

	slog.Debug("slides service created successfully", "email", email)

	return svc, nil
}
