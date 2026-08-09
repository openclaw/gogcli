package input

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/openclaw/gogcli/internal/ui"
)

func PromptLine(ctx context.Context, prompt string) (string, error) {
	return PromptLineFrom(ctx, prompt, os.Stdin)
}

func PromptLineFrom(ctx context.Context, prompt string, r io.Reader) (string, error) {
	if u := ui.FromContext(ctx); u != nil {
		u.Err().Print(prompt)
	} else {
		_, _ = fmt.Fprint(os.Stderr, prompt)
	}

	return ReadLine(r)
}
