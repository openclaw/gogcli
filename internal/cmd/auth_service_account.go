package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthServiceAccountCmd struct {
	Set    AuthServiceAccountSetCmd    `cmd:"" name:"set" help:"Store a service account key for impersonation"`
	Unset  AuthServiceAccountUnsetCmd  `cmd:"" name:"unset" help:"Remove stored service account key"`
	Status AuthServiceAccountStatusCmd `cmd:"" name:"status" help:"Show stored service account key status"`
}

type serviceAccountJSONInfo struct {
	ClientEmail string
	ClientID    string
}

func parseServiceAccountJSON(data []byte) (serviceAccountJSONInfo, error) {
	var saJSON map[string]any
	if err := json.Unmarshal(data, &saJSON); err != nil {
		return serviceAccountJSONInfo{}, fmt.Errorf("invalid service account JSON: %w", err)
	}
	if saJSON["type"] != "service_account" {
		return serviceAccountJSONInfo{}, fmt.Errorf("invalid service account JSON: expected type=service_account")
	}

	info := serviceAccountJSONInfo{}
	if v, ok := saJSON["client_email"].(string); ok {
		info.ClientEmail = strings.TrimSpace(v)
	}
	if v, ok := saJSON["client_id"].(string); ok {
		info.ClientID = strings.TrimSpace(v)
	}
	return info, nil
}

type AuthServiceAccountSetCmd struct {
	Email string `arg:"" name:"email" help:"Email to impersonate (Workspace user email)" required:""`
	Key   string `name:"key" required:"" help:"Path to service account JSON key file"`
}

func (c *AuthServiceAccountSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	keyPath := strings.TrimSpace(c.Key)
	if keyPath == "" {
		return usage("empty key path")
	}
	keyPath, err := config.ExpandPath(keyPath)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(keyPath) //nolint:gosec // user-provided path
	if err != nil {
		return fmt.Errorf("read service account key: %w", err)
	}

	info, err := parseServiceAccountJSON(data)
	if err != nil {
		return err
	}

	destPath, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	if err := dryRunExit(ctx, flags, "auth.service_account.set", map[string]any{
		"email":        email,
		"key_path":     keyPath,
		"dest_path":    destPath,
		"client_email": info.ClientEmail,
		"client_id":    info.ClientID,
	}); err != nil {
		return err
	}

	if _, err := config.EnsureDir(); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, data, 0o600); err != nil {
		return fmt.Errorf("write service account: %w", err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"stored":       true,
			"email":        email,
			"path":         destPath,
			"client_email": info.ClientEmail,
			"client_id":    info.ClientID,
		})
	}
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", destPath)
	if info.ClientEmail != "" {
		u.Out().Printf("client_email\t%s", info.ClientEmail)
	}
	if info.ClientID != "" {
		u.Out().Printf("client_id\t%s", info.ClientID)
	}
	u.Out().Println("Service account configured. Use: gog <cmd> --account " + email)
	return nil
}

type AuthServiceAccountUnsetCmd struct {
	Email string `arg:"" name:"email" help:"Email (impersonated user)" required:""`
}

func (c *AuthServiceAccountUnsetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove stored service account for %s", email)); err != nil {
		return err
	}

	path, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return writeResult(ctx, u,
				kv("deleted", false),
				kv("email", email),
				kv("path", path),
			)
		}
		return fmt.Errorf("remove service account: %w", err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("email", email),
		kv("path", path),
	)
}

type AuthServiceAccountStatusCmd struct {
	Email string `arg:"" name:"email" help:"Email (impersonated user)" required:""`
}

func (c *AuthServiceAccountStatusCmd) Run(ctx context.Context) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	path, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path) //nolint:gosec // stored in user config dir
	if err != nil {
		if os.IsNotExist(err) {
			if outfmt.IsJSON(ctx) {
				return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
					"email":   email,
					"path":    path,
					"exists":  false,
					"stored":  false,
					"message": "no service account configured",
				})
			}
			u.Out().Printf("email\t%s", email)
			u.Out().Printf("path\t%s", path)
			u.Out().Printf("exists\tfalse")
			return nil
		}
		return fmt.Errorf("read service account: %w", err)
	}

	info, parseErr := parseServiceAccountJSON(data)
	if parseErr != nil {
		return parseErr
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"email":        email,
			"path":         path,
			"exists":       true,
			"stored":       true,
			"client_email": info.ClientEmail,
			"client_id":    info.ClientID,
		})
	}
	u.Out().Printf("email\t%s", email)
	u.Out().Printf("path\t%s", path)
	u.Out().Printf("exists\ttrue")
	if info.ClientEmail != "" {
		u.Out().Printf("client_email\t%s", info.ClientEmail)
	}
	if info.ClientID != "" {
		u.Out().Printf("client_id\t%s", info.ClientID)
	}
	return nil
}
