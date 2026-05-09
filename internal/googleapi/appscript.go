package googleapi

import (
	"context"

	"google.golang.org/api/script/v1"

	"github.com/steipete/gogcli/internal/googleauth"
)

func NewAppScript(ctx context.Context, email string) (*script.Service, error) {
	return newGoogleServiceForAccount(ctx, email, googleauth.ServiceAppScript, "appscript", script.NewService)
}
