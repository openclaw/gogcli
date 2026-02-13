package googleapi

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/api/sheets/v4"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func NewSheets(ctx context.Context, email string) (*sheets.Service, error) {
	slog.Debug("creating sheets service", "email", email)

	opts, err := optionsForAccount(ctx, googleauth.ServiceSheets, email)
	if err != nil {
		return nil, fmt.Errorf("sheets options: %w", err)
	}

	svc, err := sheets.NewService(ctx, opts...)
	if err != nil {
		slog.Error("failed to create sheets service", "email", email, "error", err)
		return nil, fmt.Errorf("create sheets service: %w", err)
	}

	slog.Debug("sheets service created successfully", "email", email)

	return svc, nil
}
