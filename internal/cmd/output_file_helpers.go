package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

const stdoutPath = "-"

type outputFileOptions struct {
	Overwrite bool
	FileMode  os.FileMode
	DirMode   os.FileMode
}

func isStdoutPath(path string) bool {
	return strings.TrimSpace(path) == stdoutPath
}

func outputPathOrStdout(path string) string {
	if strings.TrimSpace(path) == "" {
		return stdoutPath
	}
	return path
}

func openUserOutputFile(path string, opts outputFileOptions) (*os.File, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, "", fmt.Errorf("output path required")
	}

	expanded, err := config.ExpandPath(path)
	if err != nil {
		return nil, "", err
	}

	dirMode := opts.DirMode
	if dirMode == 0 {
		dirMode = 0o700
	}
	if dir := filepath.Dir(expanded); dir != "." {
		// User picked the destination path; create missing parents with private perms.
		// #nosec G301,G703 -- destination directory is explicitly chosen by the caller.
		if mkdirErr := os.MkdirAll(dir, dirMode); mkdirErr != nil {
			return nil, "", mkdirErr
		}
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !opts.Overwrite {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	fileMode := opts.FileMode
	if fileMode == 0 {
		fileMode = 0o600
	}
	f, err := os.OpenFile(expanded, flags, fileMode) //nolint:gosec // user-provided output path
	if err != nil {
		return nil, "", err
	}
	return f, expanded, nil
}

func createUserOutputFile(path string) (*os.File, string, error) {
	return openUserOutputFile(path, outputFileOptions{
		Overwrite: true,
		FileMode:  0o600,
		DirMode:   0o700,
	})
}
