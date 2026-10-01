package cmd

import (
	"context"

	"github.com/openclaw/gogcli/internal/outfmt"
)

type gmailCompactSearchKey struct{}

func withGmailCompactSearch(ctx context.Context) context.Context {
	return context.WithValue(ctx, gmailCompactSearchKey{}, true)
}

func gmailCompactSearch(ctx context.Context) bool {
	enabled, _ := ctx.Value(gmailCompactSearchKey{}).(bool)
	return enabled
}

func compactGmailThreadItem(item threadItem) threadItem {
	for _, field := range []struct {
		name string
		dest *string
	}{{"from", &item.From}, {"subject", &item.Subject}, {"date", &item.Date}} {
		var cut bool
		*field.dest, cut = truncateMCPText(*field.dest, 256)
		if cut {
			item.TruncatedFields = append(item.TruncatedFields, field.name)
		}
	}
	if len(item.Labels) > 100 {
		item.Labels = item.Labels[:100]
		item.TruncatedFields = append(item.TruncatedFields, "labels")
	}
	for i, label := range item.Labels {
		var cut bool
		item.Labels[i], cut = truncateMCPText(label, 256)
		if cut {
			item.TruncatedFields = append(item.TruncatedFields, "labels")
		}
	}
	return item
}

func wrapCompactGmailThreadItems(ctx context.Context, items []threadItem) []threadItem {
	if _, ok := outfmt.UntrustedWrapperFromContext(ctx); !ok {
		return items
	}
	for i := range items {
		for _, field := range []*string{&items[i].From, &items[i].Subject, &items[i].Date} {
			if *field != "" {
				*field = outfmt.WrapUntrustedContent(*field, outfmt.UntrustedWrapOptions{Enabled: true, Source: "google_api"})
			}
		}
	}
	return items
}
