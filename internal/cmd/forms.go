package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	formsapi "google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newFormsService = googleapi.NewForms

type FormsCmd struct {
	Get       FormsGetCmd       `cmd:"" name:"get" aliases:"info,show" help:"Get a form"`
	Create    FormsCreateCmd    `cmd:"" name:"create" aliases:"new" help:"Create a form"`
	Responses FormsResponsesCmd `cmd:"" name:"responses" help:"Form responses"`
}

type FormsResponsesCmd struct {
	List FormsResponsesListCmd `cmd:"" name:"list" aliases:"ls" help:"List form responses"`
	Get  FormsResponseGetCmd   `cmd:"" name:"get" aliases:"info,show" help:"Get a form response"`
}

type FormsGetCmd struct {
	FormID string `arg:"" name:"formId" help:"Form ID"`
}

func (c *FormsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	form, err := svc.Forms.Get(formID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"form":     form,
			"edit_url": formEditURL(formID),
		})
	}

	u := ui.FromContext(ctx)
	printFormSummary(u, form, formID)
	return nil
}

type FormsCreateCmd struct {
	Title       string `name:"title" help:"Form title" required:""`
	Description string `name:"description" help:"Form description"`
}

func (c *FormsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("empty --title")
	}
	description := strings.TrimSpace(c.Description)

	if dryRunErr := dryRunExit(ctx, flags, "forms.create", map[string]any{
		"title":       title,
		"description": description,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	req := &formsapi.Form{Info: &formsapi.Info{
		Title:       title,
		Description: description,
	}}
	form, err := svc.Forms.Create(req).Context(ctx).Do()
	if err != nil {
		return err
	}

	formID := strings.TrimSpace(form.FormId)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"created":  true,
			"form":     form,
			"edit_url": formEditURL(formID),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("created\ttrue")
	printFormSummary(u, form, formID)
	return nil
}

type FormsResponsesListCmd struct {
	FormID string `arg:"" name:"formId" help:"Form ID"`
	Max    int    `name:"max" help:"Maximum responses" default:"20"`
	Page   string `name:"page" help:"Page token"`
	Filter string `name:"filter" help:"Filter expression"`
}

func (c *FormsResponsesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	if c.Max <= 0 {
		return usage("--max must be > 0")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Forms.Responses.List(formID).PageSize(int64(c.Max)).Context(ctx)
	if page := strings.TrimSpace(c.Page); page != "" {
		call = call.PageToken(page)
	}
	if filter := strings.TrimSpace(c.Filter); filter != "" {
		call = call.Filter(filter)
	}
	resp, err := call.Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"form_id":       formID,
			"responses":     resp.Responses,
			"nextPageToken": resp.NextPageToken,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Println("RESPONSE_ID\tSUBMITTED\tEMAIL")
	for _, item := range resp.Responses {
		if item == nil {
			continue
		}
		submitted := firstFormTime(item.LastSubmittedTime, item.CreateTime)
		u.Out().Printf("%s\t%s\t%s", item.ResponseId, submitted, item.RespondentEmail)
	}
	if next := strings.TrimSpace(resp.NextPageToken); next != "" {
		u.Err().Println("# Next page: --page " + next)
	}
	return nil
}

type FormsResponseGetCmd struct {
	FormID     string `arg:"" name:"formId" help:"Form ID"`
	ResponseID string `arg:"" name:"responseId" help:"Response ID"`
}

func (c *FormsResponseGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	responseID := strings.TrimSpace(c.ResponseID)
	if responseID == "" {
		return usage("empty responseId")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := svc.Forms.Responses.Get(formID, responseID).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"response": resp,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Printf("response_id\t%s", resp.ResponseId)
	u.Out().Printf("submitted\t%s", firstFormTime(resp.LastSubmittedTime, resp.CreateTime))
	if resp.RespondentEmail != "" {
		u.Out().Printf("email\t%s", resp.RespondentEmail)
	}
	u.Out().Printf("answers\t%d", len(resp.Answers))
	if resp.TotalScore != 0 {
		u.Out().Printf("total_score\t%s", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", resp.TotalScore), "0"), "."))
	}
	return nil
}

func printFormSummary(u *ui.UI, form *formsapi.Form, fallbackID string) {
	if u == nil || form == nil {
		return
	}
	formID := strings.TrimSpace(form.FormId)
	if formID == "" {
		formID = strings.TrimSpace(fallbackID)
	}
	u.Out().Printf("id\t%s", formID)
	if form.Info != nil {
		if form.Info.Title != "" {
			u.Out().Printf("title\t%s", form.Info.Title)
		}
		if form.Info.Description != "" {
			u.Out().Printf("description\t%s", form.Info.Description)
		}
	}
	if form.ResponderUri != "" {
		u.Out().Printf("responder_uri\t%s", form.ResponderUri)
	}
	u.Out().Printf("edit_url\t%s", formEditURL(formID))
}

func formEditURL(formID string) string {
	formID = strings.TrimSpace(formID)
	if formID == "" {
		return ""
	}
	return "https://docs.google.com/forms/d/" + formID + "/edit"
}

func firstFormTime(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
