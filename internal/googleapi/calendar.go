package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/calendar/v3"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func NewCalendar(ctx context.Context, email string) (*calendar.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceCalendar, email); err != nil {
		return nil, fmt.Errorf("calendar options: %w", err)
	} else if svc, err := calendar.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create calendar service: %w", err)
	} else {
		return svc, nil
	}
}
