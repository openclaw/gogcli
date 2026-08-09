package cmd

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type GmailSendAsCmd struct {
	List   GmailSendAsListCmd   `cmd:"" name:"list" aliases:"ls" help:"List send-as aliases"`
	Get    GmailSendAsGetCmd    `cmd:"" name:"get" aliases:"info,show" help:"Get details of a send-as alias"`
	Create GmailSendAsCreateCmd `cmd:"" name:"create" aliases:"add,new" help:"Create a new send-as alias"`
	Verify GmailSendAsVerifyCmd `cmd:"" name:"verify" aliases:"resend" help:"Resend verification email for a send-as alias"`
	Delete GmailSendAsDeleteCmd `cmd:"" name:"delete" aliases:"rm,del,remove" help:"Delete a send-as alias"`
	Update GmailSendAsUpdateCmd `cmd:"" name:"update" aliases:"edit,set" help:"Update a send-as alias"`
}

type GmailSendAsListCmd struct{}

const sendAsYes = "yes"

func (c *GmailSendAsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Users.Settings.SendAs.List("me").Do()
	if err != nil {
		return err
	}
	sendAs := normalizeGmailSettingsItems(resp.SendAs)

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"sendAs": sendAs})
	}

	if len(sendAs) == 0 {
		u.Err().Println("No send-as aliases")
		return nil
	}

	tw := tabwriter.NewWriter(stdoutWriter(ctx), 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "EMAIL\tDISPLAY NAME\tDEFAULT\tVERIFIED\tTREAT AS ALIAS")
	for _, sa := range sendAs {
		isDefault := ""
		if sa.IsDefault {
			isDefault = sendAsYes
		}
		verified := "pending"
		if sa.VerificationStatus == gmailVerificationAccepted {
			verified = sendAsYes
		}
		treatAsAlias := ""
		if sa.TreatAsAlias {
			treatAsAlias = sendAsYes
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			sa.SendAsEmail, sa.DisplayName, isDefault, verified, treatAsAlias)
	}
	_ = tw.Flush()
	return nil
}

type GmailSendAsGetCmd struct {
	Email string `arg:"" name:"email" help:"Send-as email"`
}

func (c *GmailSendAsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	sendAsEmail := strings.TrimSpace(c.Email)
	if sendAsEmail == "" {
		return usage("email is required")
	}
	if validateErr := validateGmailSettingsEmail("email", sendAsEmail); validateErr != nil {
		return validateErr
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	sa, err := svc.Users.Settings.SendAs.Get("me", sendAsEmail).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"sendAs": sa})
	}

	u.Out().Linef("send_as_email\t%s", sa.SendAsEmail)
	u.Out().Linef("display_name\t%s", sa.DisplayName)
	u.Out().Linef("reply_to\t%s", sa.ReplyToAddress)
	u.Out().Linef("signature\t%s", sa.Signature)
	u.Out().Linef("is_primary\t%t", sa.IsPrimary)
	u.Out().Linef("is_default\t%t", sa.IsDefault)
	u.Out().Linef("treat_as_alias\t%t", sa.TreatAsAlias)
	u.Out().Linef("verification_status\t%s", sa.VerificationStatus)
	return nil
}

type GmailSendAsCreateCmd struct {
	Email        string `arg:"" name:"email" help:"Send-as email"`
	DisplayName  string `name:"display-name" help:"Name that appears in the From field"`
	ReplyTo      string `name:"reply-to" help:"Reply-to address (optional)"`
	Signature    string `name:"signature" help:"HTML signature for emails sent from this alias"`
	TreatAsAlias bool   `name:"treat-as-alias" help:"Treat as alias (replies sent from Gmail web)" default:"true"`
}

func (c *GmailSendAsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	sendAsEmail := strings.TrimSpace(c.Email)
	if sendAsEmail == "" {
		return usage("email is required")
	}
	if err := validateGmailSettingsEmail("email", sendAsEmail); err != nil {
		return err
	}
	replyTo := strings.TrimSpace(c.ReplyTo)
	if replyTo != "" {
		if err := validateGmailSettingsEmail("--reply-to", replyTo); err != nil {
			return err
		}
	}

	sendAs := &gmail.SendAs{
		SendAsEmail:    sendAsEmail,
		DisplayName:    c.DisplayName,
		ReplyToAddress: replyTo,
		Signature:      c.Signature,
		TreatAsAlias:   c.TreatAsAlias,
	}

	if err := dryRunExit(ctx, flags, "gmail.sendas.create", map[string]any{
		"send_as": sendAs,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	created, err := svc.Users.Settings.SendAs.Create("me", sendAs).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"sendAs": created})
	}

	u.Out().Linef("send_as_email\t%s", created.SendAsEmail)
	u.Out().Linef("verification_status\t%s", created.VerificationStatus)
	u.Err().Println("Verification email sent. Check your inbox to complete setup.")
	return nil
}

type GmailSendAsVerifyCmd struct {
	Email string `arg:"" name:"email" help:"Send-as email"`
}

func (c *GmailSendAsVerifyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	sendAsEmail := strings.TrimSpace(c.Email)
	if sendAsEmail == "" {
		return usage("email is required")
	}
	if err := validateGmailSettingsEmail("email", sendAsEmail); err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "gmail.sendas.verify", map[string]any{
		"email": sendAsEmail,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.SendAs.Verify("me", sendAsEmail).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"email":   sendAsEmail,
			"message": "Verification email sent",
		})
	}

	u.Out().Linef("Verification email sent to %s", sendAsEmail)
	return nil
}

type GmailSendAsDeleteCmd struct {
	Email string `arg:"" name:"email" help:"Send-as email"`
}

func (c *GmailSendAsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	sendAsEmail := strings.TrimSpace(c.Email)
	if sendAsEmail == "" {
		return usage("email is required")
	}
	if err := validateGmailSettingsEmail("email", sendAsEmail); err != nil {
		return err
	}

	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "gmail.sendas.delete", map[string]any{
		"email": sendAsEmail,
	}, fmt.Sprintf("delete gmail send-as alias %s", sendAsEmail)); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	err = svc.Users.Settings.SendAs.Delete("me", sendAsEmail).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"email":   sendAsEmail,
			"deleted": true,
		})
	}

	u.Out().Linef("Deleted send-as alias: %s", sendAsEmail)
	return nil
}

type GmailSendAsUpdateCmd struct {
	Email        string `arg:"" name:"email" help:"Send-as email"`
	DisplayName  string `name:"display-name" help:"Name that appears in the From field"`
	ReplyTo      string `name:"reply-to" help:"Reply-to address"`
	Signature    string `name:"signature" help:"HTML signature"`
	TreatAsAlias bool   `name:"treat-as-alias" help:"Treat as alias" default:"true"`
	MakeDefault  bool   `name:"make-default" help:"Make this the default send-as address"`
}

func (c *GmailSendAsUpdateCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	sendAsEmail := strings.TrimSpace(c.Email)
	if sendAsEmail == "" {
		return usage("email is required")
	}
	if err := validateGmailSettingsEmail("email", sendAsEmail); err != nil {
		return err
	}
	replyTo := strings.TrimSpace(c.ReplyTo)
	if flagProvided(kctx, "reply-to") && replyTo != "" {
		if err := validateGmailSettingsEmail("--reply-to", replyTo); err != nil {
			return err
		}
	}

	updates := map[string]any{}
	if flagProvided(kctx, "display-name") {
		updates["display_name"] = c.DisplayName
	}
	if flagProvided(kctx, "reply-to") {
		updates["reply_to"] = replyTo
	}
	if flagProvided(kctx, "signature") {
		updates["signature"] = c.Signature
	}
	if flagProvided(kctx, "treat-as-alias") {
		updates["treat_as_alias"] = c.TreatAsAlias
	}
	if flagProvided(kctx, "make-default") {
		updates["make_default"] = c.MakeDefault
	}

	if err := dryRunExit(ctx, flags, "gmail.sendas.update", map[string]any{
		"email":   sendAsEmail,
		"updates": updates,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := gmailService(ctx, account)
	if err != nil {
		return err
	}

	// Get current settings first
	current, err := svc.Users.Settings.SendAs.Get("me", sendAsEmail).Do()
	if err != nil {
		return err
	}

	// Update only provided fields
	if flagProvided(kctx, "display-name") {
		current.DisplayName = c.DisplayName
	}
	if flagProvided(kctx, "reply-to") {
		current.ReplyToAddress = replyTo
	}
	if flagProvided(kctx, "signature") {
		current.Signature = c.Signature
	}
	if flagProvided(kctx, "treat-as-alias") {
		current.TreatAsAlias = c.TreatAsAlias
	}
	if flagProvided(kctx, "make-default") {
		current.IsDefault = c.MakeDefault
	}

	updated, err := svc.Users.Settings.SendAs.Update("me", sendAsEmail, current).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"sendAs": updated})
	}

	u.Out().Linef("Updated send-as alias: %s", updated.SendAsEmail)
	return nil
}
