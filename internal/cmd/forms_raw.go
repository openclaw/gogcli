package cmd

import (
	"context"
	"net/url"
	"strings"
)

// FormsRawCmd dumps the full Forms.Get response as JSON.
//
// REST reference: https://developers.google.com/forms/api/reference/rest/v1/forms/get
// Go type: https://pkg.go.dev/google.golang.org/api/forms/v1#Form
type FormsRawCmd struct {
	FormID string `arg:"" name:"formId" help:"Form ID"`
	Pretty bool   `name:"pretty" help:"Pretty-print JSON (default: compact single-line)"`
}

func (c *FormsRawCmd) Run(ctx context.Context, flags *RootFlags) error {
	formID := strings.TrimSpace(c.FormID)
	if formID == "" {
		return usage("empty formId")
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	client, err := formsHTTPClient(ctx, account)
	if err != nil {
		return err
	}

	form, err := readRawObject(ctx, client, "https://forms.googleapis.com/v1/forms/"+url.PathEscape(formID), nil, "form")
	if err != nil {
		return err
	}

	return writeRawJSON(ctx, form, c.Pretty)
}
