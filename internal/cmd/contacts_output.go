package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func writeDeleteResult(ctx context.Context, u *ui.UI, resourceName string) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"deleted": true, "resource": resourceName})
	}
	if u == nil {
		_, _ = fmt.Fprintf(os.Stdout, "deleted\ttrue\nresource\t%s\n", resourceName)
		return nil
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("resource\t%s", resourceName)
	return nil
}
