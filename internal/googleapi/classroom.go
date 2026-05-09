package googleapi

import (
	"context"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewClassroom(ctx context.Context, email string) (*classroom.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceClassroom, "classroom", classroom.NewService)
}
