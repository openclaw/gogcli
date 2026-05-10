package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

func resolveBodyInput(body, bodyFile string) (string, error) {
	return resolveBodyFileInput(body, bodyFile, "--body", "--body-file")
}

func resolveBodyFileInput(body, bodyFile, bodyFlag, bodyFileFlag string) (string, error) {
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
