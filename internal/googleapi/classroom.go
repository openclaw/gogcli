package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewClassroom(ctx context.Context, email string) (*classroom.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceClassroom, email); err != nil {
		return nil, fmt.Errorf("classroom options: %w", err)
	} else if svc, err := classroom.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create classroom service: %w", err)
	} else {
		return svc, nil
	}
}
