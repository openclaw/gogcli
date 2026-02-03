package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newFormsService = googleapi.NewForms

type FormsCmd struct {
	List      FormsListCmd      `cmd:"" name:"list" aliases:"ls" help:"List forms"`
	Get       FormsGetCmd       `cmd:"" name:"get" help:"Get form"`
	Create    FormsCreateCmd    `cmd:"" name:"create" help:"Create form"`
	Responses FormsResponsesCmd `cmd:"" name:"responses" help:"List form responses"`
}

type FormsListCmd struct {
	User string `name:"user" help:"User email to list forms for"`
	Max  int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page string `name:"page" help:"Page token"`
}

func (c *FormsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if strings.TrimSpace(c.User) != "" {
		account = strings.TrimSpace(c.User)
	}

	svc, err := newDriveService(ctx, account)
	if err != nil {
		return err
	}

	query := "mimeType='application/vnd.google-apps.form' and trashed=false"
	call := svc.Files.List().Q(query).Fields("files(id,name,owners(emailAddress),createdTime),nextPageToken")
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list forms: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Files) == 0 {
		u.Err().Println("No forms found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tNAME\tOWNER\tCREATED")
	for _, file := range resp.Files {
		if file == nil {
			continue
		}
		owner := ""
		if len(file.Owners) > 0 {
			owner = file.Owners[0].EmailAddress
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(file.Id),
			sanitizeTab(file.Name),
			sanitizeTab(owner),
			sanitizeTab(file.CreatedTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type FormsGetCmd struct {
	FormID string `arg:"" name:"form-id" help:"Form ID"`
}

func (c *FormsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	formID := strings.TrimSpace(c.FormID)
	if formID == "" {
		return usage("form ID is required")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	form, err := svc.Forms.Get(formID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("get form %s: %w", formID, err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, form)
	}

	title := ""
	if form.Info != nil {
		title = form.Info.Title
	}
	fmt.Fprintf(os.Stdout, "ID:    %s\n", form.FormId)
	fmt.Fprintf(os.Stdout, "Title: %s\n", title)
	if form.ResponderUri != "" {
		fmt.Fprintf(os.Stdout, "Responder URL: %s\n", form.ResponderUri)
	}
	return nil
}

type FormsCreateCmd struct {
	Title string `name:"title" help:"Form title" required:""`
}

func (c *FormsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	title := strings.TrimSpace(c.Title)
	if title == "" {
		return usage("--title is required")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	form := &forms.Form{Info: &forms.Info{Title: title}}
	created, err := svc.Forms.Create(form).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("create form: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	u.Out().Printf("Created form: %s (%s)\n", created.FormId, title)
	return nil
}

type FormsResponsesCmd struct {
	FormID string `arg:"" name:"form-id" help:"Form ID"`
	Max    int64  `name:"max" aliases:"limit" default:"100" help:"Max results"`
	Page   string `name:"page" help:"Page token"`
}

func (c *FormsResponsesCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	formID := strings.TrimSpace(c.FormID)
	if formID == "" {
		return usage("form ID is required")
	}

	svc, err := newFormsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Forms.Responses.List(formID)
	if c.Max > 0 {
		call = call.PageSize(c.Max)
	}
	if c.Page != "" {
		call = call.PageToken(c.Page)
	}

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("list responses: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, resp)
	}

	if len(resp.Responses) == 0 {
		u.Err().Println("No responses found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "RESPONSE ID\tEMAIL\tCREATED\tLAST SUBMITTED")
	for _, response := range resp.Responses {
		if response == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(response.ResponseId),
			sanitizeTab(response.RespondentEmail),
			sanitizeTab(response.CreateTime),
			sanitizeTab(response.LastSubmittedTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}
