package googleapi

import (
	"context"
	"fmt"

	"google.golang.org/api/tasks/v1"

	"github.com/degree-analytics/ratatosk/internal/googleauth"
)

func NewTasks(ctx context.Context, email string) (*tasks.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceTasks, email); err != nil {
		return nil, fmt.Errorf("tasks options: %w", err)
	} else if svc, err := tasks.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create tasks service: %w", err)
	} else {
		return svc, nil
	}
}
