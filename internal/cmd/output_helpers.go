package cmd

import (
	"context"
	"io"
	"os"
	"text/tabwriter"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type resultKV struct {
	Key   string
	Value any
}

func kv(key string, value any) resultKV {
	return resultKV{Key: key, Value: value}
}

func tableWriter(ctx context.Context) (io.Writer, func()) {
	if outfmt.IsPlain(ctx) {
		return os.Stdout, func() {}
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	return tw, func() { _ = tw.Flush() }
}

func writeResult(ctx context.Context, u *ui.UI, kvs ...resultKV) error {
	if outfmt.IsJSON(ctx) {
		m := make(map[string]any, len(kvs))
		for _, kv := range kvs {
			m[kv.Key] = kv.Value
		}
		return outfmt.WriteJSON(ctx, os.Stdout, m)
	}
	if u == nil {
		return nil
	}
	for _, kv := range kvs {
		switch v := kv.Value.(type) {
		case bool:
			u.Out().Printf("%s\t%t", kv.Key, v)
		default:
			u.Out().Printf("%s\t%v", kv.Key, kv.Value)
		}
	}
	return nil
}

func printNextPageHint(u *ui.UI, nextPageToken string) {
	if u == nil || nextPageToken == "" {
		return
	}
	u.Err().Printf("# Next page: --page %s", nextPageToken)
}
