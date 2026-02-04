package googleapi

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/googleauth"
)

// EscapeDriveQueryValue escapes a value for safe interpolation into a
// Google Drive API query string.  At minimum it handles backslashes and
// single quotes which are the two characters with special meaning inside
// single-quoted literals in the Drive query grammar.
func EscapeDriveQueryValue(s string) string {
	// Escape backslashes first, then single quotes.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

func NewDrive(ctx context.Context, email string) (*drive.Service, error) {
	if opts, err := optionsForAccount(ctx, googleauth.ServiceDrive, email); err != nil {
		return nil, fmt.Errorf("drive options: %w", err)
	} else if svc, err := drive.NewService(ctx, opts...); err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	} else {
		return svc, nil
	}
}
