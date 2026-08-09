package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"google.golang.org/api/drive/v3"

	"github.com/openclaw/gogcli/internal/config"
)

func resolveDriveDownloadDestPath(meta *drive.File, outPathFlag, defaultDir string) (string, error) {
	if meta == nil {
		return "", errors.New("missing file metadata")
	}
	if strings.TrimSpace(meta.Id) == "" {
		return "", errors.New("file has no id")
	}
	if strings.TrimSpace(meta.Name) == "" {
		return "", errors.New("file has no name")
	}

	destPath := strings.TrimSpace(outPathFlag)
	if isStdoutPath(destPath) {
		return stdoutPath, nil
	}
	// Expand ~ to home directory (shell doesn't expand when path is quoted).
	if destPath != "" {
		expanded, err := config.ExpandPath(destPath)
		if err != nil {
			return "", err
		}
		destPath = expanded
	}
	// Sanitize filename to prevent path traversal.
	safeName := filepath.Base(meta.Name)
	if safeName == "" || safeName == "." || safeName == ".." {
		safeName = "download"
	}
	defaultName := fmt.Sprintf("%s_%s", meta.Id, safeName)

	if destPath == "" {
		if strings.TrimSpace(defaultDir) == "" {
			return "", errors.New("missing default downloads directory")
		}
		return filepath.Join(defaultDir, defaultName), nil
	}

	if st, err := os.Stat(destPath); err == nil && st.IsDir() {
		return filepath.Join(destPath, defaultName), nil
	}
	return destPath, nil
}
