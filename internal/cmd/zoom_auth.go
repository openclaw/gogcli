package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/steipete/gogcli/internal/input"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
	"github.com/steipete/gogcli/internal/zoom"
)

type ZoomCmd struct {
	Auth ZoomAuthCmd `cmd:"" name:"auth" help:"Manage Zoom Server-to-Server OAuth credentials"`
}

type ZoomAuthCmd struct {
	Setup  ZoomAuthSetupCmd  `cmd:"" name:"setup" help:"Store Zoom Server-to-Server OAuth credentials"`
	Doctor ZoomAuthDoctorCmd `cmd:"" name:"doctor" help:"Validate Zoom credentials"`
}

type ZoomAuthSetupCmd struct {
	Alias        string `name:"alias" help:"Zoom credential alias" default:"default"`
	AccountID    string `name:"account-id" help:"Zoom Server-to-Server OAuth account ID" env:"GOG_ZOOM_ACCOUNT_ID"`
	ClientID     string `name:"client-id" help:"Zoom Server-to-Server OAuth client ID" env:"GOG_ZOOM_CLIENT_ID"`
	ClientSecret string `name:"client-secret" help:"Zoom Server-to-Server OAuth client secret" env:"GOG_ZOOM_CLIENT_SECRET"`
	SkipValidate bool   `name:"skip-validate" help:"Store credentials without calling Zoom /users/me"`
}

var defaultZoomScopes = []string{"meeting:write", "meeting:read", "user:read"}

func (c *ZoomAuthSetupCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	alias := zoom.NormalizeAlias(c.Alias)
	if flags != nil && flags.NoInput && (strings.TrimSpace(c.AccountID) == "" || strings.TrimSpace(c.ClientID) == "" || strings.TrimSpace(c.ClientSecret) == "") {
		return usage("provide --account-id, --client-id, and --client-secret with --no-input")
	}
	if existing, err := zoom.LoadMetadata(alias); err == nil && flags != nil && !flags.Force {
		return usage(fmt.Sprintf("Zoom credentials for alias %q already exist (account_id=%s); use --force to overwrite", alias, existing.AccountID))
	}
	accountID, err := promptDefault(ctx, "Zoom account ID: ", c.AccountID)
	if err != nil {
		return err
	}
	clientID, err := promptDefault(ctx, "Zoom client ID: ", c.ClientID)
	if err != nil {
		return err
	}
	clientSecret := strings.TrimSpace(c.ClientSecret)
	if clientSecret == "" {
		clientSecret, err = promptSecret("Zoom client secret: ")
		if err != nil {
			return err
		}
	}
	creds := zoom.Credentials{AccountID: accountID, ClientID: clientID, ClientSecret: clientSecret}
	if !c.SkipValidate {
		client, clientErr := zoom.NewClient(alias, creds)
		if clientErr != nil {
			return clientErr
		}
		if validateErr := client.Validate(ctx); validateErr != nil {
			return fmt.Errorf("validate Zoom credentials: %w", validateErr)
		}
	}
	if err := zoom.StoreCredentials(alias, zoom.Metadata{
		AccountID: accountID,
		ClientID:  clientID,
		Scopes:    defaultZoomScopes,
	}, clientSecret); err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"saved":  true,
			"alias":  alias,
			"scopes": defaultZoomScopes,
		})
	}
	if u != nil {
		u.Out().Linef("saved\ttrue")
		u.Out().Linef("alias\t%s", alias)
		u.Out().Linef("scopes\t%s", strings.Join(defaultZoomScopes, ","))
	}
	return nil
}

type ZoomAuthDoctorCmd struct {
	Alias string `name:"alias" help:"Zoom credential alias" default:"default"`
}

func (c *ZoomAuthDoctorCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	alias := zoom.NormalizeAlias(c.Alias)
	checks := make([]authDoctorCheck, 0)
	add := func(name, status, detail, hint string) {
		checks = append(checks, authDoctorCheck{Name: name, Status: status, Detail: detail, Hint: hint})
	}
	creds, err := zoom.LoadCredentials(alias)
	if err != nil {
		add("zoom.credentials", doctorError, err.Error(), "run `gog zoom auth setup`")
		return writeZoomDoctorResult(ctx, u, checks)
	}
	add("zoom.credentials", doctorOK, "loaded", "")
	if zoom.EnvClientSecretSet() {
		add("zoom.env_secret", doctorWarn, "GOG_ZOOM_CLIENT_SECRET is set", "environment secrets can be visible to same-user processes; prefer `gog zoom auth setup`")
	}
	if expiresAt, ok := zoom.CachedTokenExpiry(alias); ok {
		add("zoom.token_cache", doctorOK, expiresAt.UTC().Format(time.RFC3339), "")
	} else {
		add("zoom.token_cache", doctorWarn, "no cached token", "a token will be fetched on first Zoom API call")
	}
	client, clientErr := zoom.NewClient(alias, creds)
	if clientErr != nil {
		add("zoom.client", doctorError, clientErr.Error(), "")
		return writeZoomDoctorResult(ctx, u, checks)
	}
	if validateErr := client.Validate(ctx); validateErr != nil {
		add("zoom.validate", doctorError, validateErr.Error(), "verify account ID, client ID, client secret, and scopes")
	} else {
		add("zoom.validate", doctorOK, "Zoom /users/me succeeded", "")
	}
	return writeZoomDoctorResult(ctx, u, checks)
}

func writeZoomDoctorResult(ctx context.Context, u *ui.UI, checks []authDoctorCheck) error {
	status := authDoctorStatus(checks)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"status": status, "checks": checks})
	}
	if u != nil {
		for _, check := range checks {
			u.Out().Linef("%s\t%s\t%s", check.Status, check.Name, check.Detail)
			if check.Hint != "" {
				u.Out().Linef("hint\t%s\t%s", check.Name, check.Hint)
			}
		}
		u.Out().Linef("status\t%s", status)
	}
	return nil
}

func promptDefault(ctx context.Context, prompt, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		return value, nil
	}
	line, err := input.PromptLine(ctx, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptSecret(prompt string) (string, error) {
	_, _ = fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd())) //nolint:gosec // os file descriptor fits int on supported targets
	_, _ = fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}
