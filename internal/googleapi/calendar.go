package googleapi

import (
	"context"

	"google.golang.org/api/calendar/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewCalendar(ctx context.Context, email string) (*calendar.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceCalendar, "calendar", calendar.NewService)
}
