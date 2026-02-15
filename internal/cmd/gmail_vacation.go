package cmd

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailVacationCmd struct {
	Get    GmailVacationGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get current vacation responder settings"`
	Update GmailVacationUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update vacation responder settings"`
}

type GmailVacationGetCmd struct{}

func (c *GmailVacationGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	vacation, err := svc.Users.Settings.GetVacation("me").Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"vacation": vacation})
	}

	u.Out().Printf("enable_auto_reply\t%t", vacation.EnableAutoReply)
	u.Out().Printf("response_subject\t%s", vacation.ResponseSubject)
	u.Out().Printf("response_body_html\t%s", vacation.ResponseBodyHtml)
	u.Out().Printf("response_body_plain_text\t%s", vacation.ResponseBodyPlainText)
	if vacation.StartTime != 0 {
		u.Out().Printf("start_time\t%d", vacation.StartTime)
	}
	if vacation.EndTime != 0 {
		u.Out().Printf("end_time\t%d", vacation.EndTime)
	}
	u.Out().Printf("restrict_to_contacts\t%t", vacation.RestrictToContacts)
	u.Out().Printf("restrict_to_domain\t%t", vacation.RestrictToDomain)
	return nil
}

type GmailVacationUpdateCmd struct {
	Enable       bool   `name:"enable" help:"Enable vacation responder"`
	Disable      bool   `name:"disable" help:"Disable vacation responder"`
	Subject      string `name:"subject" help:"Subject line for auto-reply"`
	Body         string `name:"body" help:"HTML body of the auto-reply message"`
	Start        string `name:"start" help:"Start time in RFC3339 format (e.g., 2024-12-20T00:00:00Z)"`
	End          string `name:"end" help:"End time in RFC3339 format (e.g., 2024-12-31T23:59:59Z)"`
	ContactsOnly bool   `name:"contacts-only" help:"Only respond to contacts"`
	DomainOnly   bool   `name:"domain-only" help:"Only respond to same domain"`
}

func (c *GmailVacationUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Enable && c.Disable {
		return errors.New("cannot specify both --enable and --disable")
	}

	var err error
	updates := map[string]any{}
	if c.Enable {
		updates["enable_auto_reply"] = true
	}
	if c.Disable {
		updates["enable_auto_reply"] = false
	}
	if flagProvided(kctx, "subject") {
		updates["response_subject"] = c.Subject
	}
	if flagProvided(kctx, "body") {
		updates["response_body_html"] = c.Body
		updates["response_body_plain_text"] = stripHTML(c.Body)
	}
	if flagProvided(kctx, "start") {
		var t int64
		t, err = parseRFC3339ToMillis(c.Start)
		if err != nil {
			return err
		}
		updates["start_time"] = t
	}
	if flagProvided(kctx, "end") {
		var t int64
		t, err = parseRFC3339ToMillis(c.End)
		if err != nil {
			return err
		}
		updates["end_time"] = t
	}
	if flagProvided(kctx, "contacts-only") {
		updates["restrict_to_contacts"] = c.ContactsOnly
	}
	if flagProvided(kctx, "domain-only") {
		updates["restrict_to_domain"] = c.DomainOnly
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.vacation.update", map[string]any{
		"updates": updates,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	// Get current settings first
	current, err := svc.Users.Settings.GetVacation("me").Do()
	if err != nil {
		return err
	}

	// Build update request, preserving existing values if not specified
	vacation := &gmail.VacationSettings{
		EnableAutoReply:       current.EnableAutoReply,
		ResponseSubject:       current.ResponseSubject,
		ResponseBodyHtml:      current.ResponseBodyHtml,
		ResponseBodyPlainText: current.ResponseBodyPlainText,
		StartTime:             current.StartTime,
		EndTime:               current.EndTime,
		RestrictToContacts:    current.RestrictToContacts,
		RestrictToDomain:      current.RestrictToDomain,
	}

	// Apply flags
	if c.Enable {
		vacation.EnableAutoReply = true
	}
	if c.Disable {
		vacation.EnableAutoReply = false
	}
	if flagProvided(kctx, "subject") {
		vacation.ResponseSubject = c.Subject
	}
	if flagProvided(kctx, "body") {
		vacation.ResponseBodyHtml = c.Body
		vacation.ResponseBodyPlainText = stripHTML(c.Body)
	}
	if flagProvided(kctx, "start") {
		var t int64
		t, err = parseRFC3339ToMillis(c.Start)
		if err != nil {
			return err
		}
		vacation.StartTime = t
	}
	if flagProvided(kctx, "end") {
		var t int64
		t, err = parseRFC3339ToMillis(c.End)
		if err != nil {
			return err
		}
		vacation.EndTime = t
	}
	if flagProvided(kctx, "contacts-only") {
		vacation.RestrictToContacts = c.ContactsOnly
	}
	if flagProvided(kctx, "domain-only") {
		vacation.RestrictToDomain = c.DomainOnly
	}

	updated, err := svc.Users.Settings.UpdateVacation("me", vacation).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"vacation": updated})
	}

	u.Out().Println("Vacation responder updated successfully")
	u.Out().Printf("enable_auto_reply\t%t", updated.EnableAutoReply)
	u.Out().Printf("response_subject\t%s", updated.ResponseSubject)
	if updated.StartTime != 0 {
		u.Out().Printf("start_time\t%d", updated.StartTime)
	}
	if updated.EndTime != 0 {
		u.Out().Printf("end_time\t%d", updated.EndTime)
	}
	u.Out().Printf("restrict_to_contacts\t%t", updated.RestrictToContacts)
	u.Out().Printf("restrict_to_domain\t%t", updated.RestrictToDomain)
	return nil
}

func parseRFC3339ToMillis(rfc3339 string) (int64, error) {
	if rfc3339 == "" {
		return 0, nil
	}
	// Parse RFC3339 format and convert to milliseconds since epoch
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return 0, err
	}
	return t.UnixMilli(), nil
}

func stripHTML(html string) string {
	// Very basic HTML stripping for plain text fallback
	inTag := false
	out := make([]rune, 0, len(html))
	for _, r := range html {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			out = append(out, r)
		}
	}
	return string(out)
}
