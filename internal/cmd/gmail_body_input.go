package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

func resolveBodyInput(body, bodyFile string) (string, error) {
	bodyFile = strings.TrimSpace(bodyFile)
	if bodyFile == "" {
		return body, nil
	}
	if strings.TrimSpace(body) != "" {
		return "", usage("use only one of --body or --body-file")
	}

	var (
		b   []byte
		err error
	)
	if bodyFile == "-" {
		b, err = io.ReadAll(os.Stdin)
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

func resolveBodyHTMLInput(bodyHTML, bodyHTMLFile string) (string, error) {
	bodyHTMLFile = strings.TrimSpace(bodyHTMLFile)
	if bodyHTMLFile == "" {
		return bodyHTML, nil
	}
	if strings.TrimSpace(bodyHTML) != "" {
		return "", usage("use only one of --body-html or --body-html-file")
	}

	var (
		b   []byte
		err error
	)
	if bodyHTMLFile == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		bodyHTMLFile, err = config.ExpandPath(bodyHTMLFile)
		if err != nil {
			return "", err
		}
		b, err = os.ReadFile(bodyHTMLFile) //nolint:gosec // user-provided path
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}
