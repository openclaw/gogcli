package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
)

// resolveInlineOrFileBytes supports agent-friendly inputs for flags that otherwise
// require shell-escaped JSON strings.
//
// Supported forms:
//   - literal: '{"a":1}'
//   - stdin:   '-'
//   - file:    '@path/to/file.json'
//   - stdin:   '@-'
func resolveInlineOrFileBytes(spec string) ([]byte, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}

	readStdin := func() ([]byte, error) {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		return b, nil
	}

	switch {
	case spec == "-":
		return readStdin()
	case strings.HasPrefix(spec, "@"):
		path := strings.TrimSpace(strings.TrimPrefix(spec, "@"))
		if path == "" {
			return nil, fmt.Errorf("empty @file reference")
		}
		if path == "-" {
			return readStdin()
		}
		path, err := config.ExpandPath(path)
		if err != nil {
			return nil, err
		}
		b, err := os.ReadFile(path) //nolint:gosec // user-provided path
		if err != nil {
			return nil, err
		}
		return b, nil
	default:
		return []byte(spec), nil
	}
}
