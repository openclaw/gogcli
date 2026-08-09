package googleapi

import (
	"context"

	"google.golang.org/api/tasks/v1"

	"github.com/openclaw/gogcli/internal/googleauth"
)

func NewTasks(ctx context.Context, email string) (*tasks.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceTasks, "tasks", tasks.NewService)
}
