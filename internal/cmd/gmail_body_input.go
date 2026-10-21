package cmd

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/openclaw/gogcli/internal/config"
)

func resolveBodyInput(ctx context.Context, body, bodyFile string) (string, error) {
	return resolveBodyFileInput(ctx, body, bodyFile, "--body", "--body-file")
}

func resolveComposeBodyInputs(ctx context.Context, body, bodyFile, bodyHTML, bodyHTMLFile string) (string, string, error) {
	return resolveTextAndHTMLInputs(ctx, "--body", body, bodyFile, bodyHTML, bodyHTMLFile)
}

// resolveTextAndHTMLInputs resolves a flag family named flag, flag-file,
// flag-html and flag-html-file (--body…, --note…) into its plain and HTML text.
func resolveTextAndHTMLInputs(ctx context.Context, flag, text, textFile, textHTML, textHTMLFile string) (string, string, error) {
	if strings.TrimSpace(textFile) == "-" && strings.TrimSpace(textHTMLFile) == "-" {
		return "", "", usage("use stdin for only one of " + flag + "-file or " + flag + "-html-file")
	}

	plain, err := resolveBodyFileInput(ctx, text, textFile, flag, flag+"-file")
	if err != nil {
		return "", "", err
	}
	html, err := resolveBodyFileInput(ctx, textHTML, textHTMLFile, flag+"-html", flag+"-html-file")
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
