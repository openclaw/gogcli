package cmd

import (
	"context"
	"os"
	"strings"

	formsapi "google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// FormsPublishCmd publishes a form via forms.setPublishSettings.
type FormsPublishCmd struct {
	FormID             string `arg:"" name:"formId" help:"Form ID"`
	Unpublish          bool   `name:"unpublish" help:"Unpublish the form instead of publishing it"`
	AcceptingResponses bool   `name:"accepting-responses" help:"Whether a published form accepts responses" default:"true"`
}

func (c *FormsPublishCmd) Run(ctx context.Context, flags *RootFlags) error {
	published := !c.Unpublish
	acceptingResponses := c.AcceptingResponses
	if !published {
		acceptingResponses = false
	}
	operation := "forms.publish"
	if !published {
		operation = "forms.unpublish"
	}

	return setFormPublishState(ctx, flags, formPublishStateRequest{
		FormID:             c.FormID,
		Published:          published,
		AcceptingResponses: acceptingResponses,
		Operation:          operation,
	})
}

type formPublishStateRequest struct {
	FormID             string
	Published          bool
	AcceptingResponses bool
	Operation          string
}

func setFormPublishState(ctx context.Context, flags *RootFlags, publishReq formPublishStateRequest) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(publishReq.FormID))
	if formID == "" {
		return usage("empty formId")
	}

	if dryRunErr := dryRunExit(ctx, flags, publishReq.Operation, map[string]any{
		"form_id":             formID,
		"published":           publishReq.Published,
		"accepting_responses": publishReq.AcceptingResponses,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	req := &formsapi.SetPublishSettingsRequest{
		UpdateMask: "publish_state",
		PublishSettings: &formsapi.PublishSettings{
			PublishState: &formsapi.PublishState{
				IsPublished:          publishReq.Published,
				IsAcceptingResponses: publishReq.AcceptingResponses,
				ForceSendFields:      []string{"IsPublished", "IsAcceptingResponses"},
			},
		},
	}
	resp, err := svc.Forms.SetPublishSettings(formID, req).Context(ctx).Do()
	if err != nil {
		return err
	}

	form, err := svc.Forms.Get(formID).Context(ctx).Do()
	if err != nil {
		return err
	}

	responderURI := strings.TrimSpace(form.ResponderUri)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"published":           publishReq.Published,
			"accepting_responses": publishReq.AcceptingResponses,
			"form_id":             formID,
			"responder_uri":       responderURI,
			"edit_url":            formEditURL(formID),
			"publish_settings":    resp.PublishSettings,
			"form":                form,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("published\t%t", publishReq.Published)
	u.Out().Printf("accepting_responses\t%t", publishReq.AcceptingResponses)
	u.Out().Printf("form_id\t%s", formID)
	if responderURI != "" {
		u.Out().Printf("responder_uri\t%s", responderURI)
	}
	u.Out().Printf("edit_url\t%s", formEditURL(formID))
	return nil
}
