package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type resultKV struct {
	Key   string
	Value any
}

func kv(key string, value any) resultKV {
	return resultKV{Key: key, Value: value}
}

func tableWriter(ctx context.Context) (io.Writer, func()) {
	stdout := stdoutWriter(ctx)
	if outfmt.IsPlain(ctx) {
		return stdout, func() {}
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	return tw, func() { _ = tw.Flush() }
}

func writeResult(ctx context.Context, u *ui.UI, kvs ...resultKV) error {
	if outfmt.IsJSON(ctx) {
		m := make(map[string]any, len(kvs))
		for _, kv := range kvs {
			m[kv.Key] = kv.Value
		}
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), m)
	}
	if u == nil {
		return nil
	}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case bool:
			u.Out().Linef("%s\t%t", kv.Key, v)
		default:
			u.Out().Linef("%s\t%v", kv.Key, kv.Value)
		}
	}
	return nil
}

func printNextPageHint(u *ui.UI, nextPageToken string) {
	if u == nil || nextPageToken == "" {
		return
	}
	u.Err().Linef("# Next page: --page %s", nextPageToken)
}

func printNextPageHintWithAll(u *ui.UI, nextPageToken string, allFlag string) {
	if u == nil || nextPageToken == "" {
		return
	}
	u.Err().Linef("# More results: use %s to fetch every page, or --page %s for the next page", allFlag, nextPageToken)
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	// Keep output parseable in tables/TSV.
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func escapeTerminalControls(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	for _, current := range value {
		if unicode.IsControl(current) || unicode.Is(unicode.Cf, current) {
			switch {
			case current <= 0xff:
				fmt.Fprintf(&output, "\\x%02x", current)
			case current <= 0xffff:
				fmt.Fprintf(&output, "\\u%04x", current)
			default:
				fmt.Fprintf(&output, "\\U%08x", current)
			}
			continue
		}
		output.WriteRune(current)
	}
	return output.String()
}
