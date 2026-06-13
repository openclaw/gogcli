package cmd

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

func resolveBodyInput(ctx context.Context, body, bodyFile string) (string, error) {
	return resolveBodyFileInput(ctx, body, bodyFile, "--body", "--body-file")
}

func resolveComposeBodyInputs(ctx context.Context, body, bodyFile, bodyHTML, bodyHTMLFile string) (string, string, error) {
	if strings.TrimSpace(bodyFile) == "-" && strings.TrimSpace(bodyHTMLFile) == "-" {
		return "", "", usage("use stdin for only one of --body-file or --body-html-file")
	}

	plain, err := resolveBodyInput(ctx, body, bodyFile)
	if err != nil {
		return "", "", err
	}
	html, err := resolveBodyFileInput(ctx, bodyHTML, bodyHTMLFile, "--body-html", "--body-html-file")
	if err != nil {
		return "", "", err
	}
	return plain, html, nil
}

func resolveBodyFileInput(ctx context.Context, body, bodyFile, bodyFlag, bodyFileFlag string) (string, error) {
	bodyFile = strings.TrimSpace(bodyFile)
	if bodyFile == "" {
		return body, nil
	}
	if strings.TrimSpace(body) != "" {
		return "", usage("use only one of " + bodyFlag + " or " + bodyFileFlag)
	}

	var (
		b   []byte
		err error
	)
	if bodyFile == "-" {
		b, err = io.ReadAll(stdinReader(ctx))
	} else {
		bodyFile, err = config.ExpandPath(bodyFile)
		if err != nil {
			return "", err
		}
		b, err = os.ReadFile(bodyFile) //nolint:gosec // user-provided path
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}
