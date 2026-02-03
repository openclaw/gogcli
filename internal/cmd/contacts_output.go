package cmd

import (
	"context"
	"os"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

func writeDeleteResult(ctx context.Context, u *ui.UI, resourceName string) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"deleted": true, "resource": resourceName})
	}
	out := u
	if out == nil {
		out = ui.FromContext(ctx)
	}
	out.Out().Printf("deleted\ttrue")
	out.Out().Printf("resource\t%s", resourceName)
	return nil
}
