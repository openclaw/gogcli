package googleapi

import (
	"context"

	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewSlides(ctx context.Context, email string) (*slides.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceSlides, "slides", slides.NewService)
}
