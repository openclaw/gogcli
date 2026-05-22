package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	Email    string `arg:"" name:"email" help:"Email to impersonate (Workspace user email)" required:""`
	Key      string `name:"key" help:"Path to service account JSON key file, or '-' for stdin"`
	KeyStdin bool   `name:"key-stdin" help:"Read service account JSON key from stdin"`
	KeyEnv   string `name:"key-env" help:"Read service account JSON key from the named environment variable"`
}

func (c *AuthServiceAccountSetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	email := strings.TrimSpace(c.Email)
	if email == "" {
		return usage("empty email")
	}

	data, keySource, err := c.resolveServiceAccountKey()
	if err != nil {
		return err
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
		"key_source":   keySource,
		"dest_path":    destPath,
		"client_email": info.ClientEmail,
		"client_id":    info.ClientID,
	}); err != nil {
		return err
	}

	if _, err := config.EnsureDataDir(); err != nil {
		return err
	}
	if err := config.WriteFileAtomic(destPath, data, 0o600); err != nil {
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
	u.Out().Linef("email\t%s", email)
	u.Out().Linef("path\t%s", destPath)
	if info.ClientEmail != "" {
		u.Out().Linef("client_email\t%s", info.ClientEmail)
	}
	if info.ClientID != "" {
		u.Out().Linef("client_id\t%s", info.ClientID)
	}
	u.Out().Println("Service account configured. Use: gog <cmd> --account " + email)
	return nil
}

func (c *AuthServiceAccountSetCmd) resolveServiceAccountKey() ([]byte, string, error) {
	keyPath := strings.TrimSpace(c.Key)
	keyEnv := strings.TrimSpace(c.KeyEnv)

	sources := 0
	if keyPath != "" {
		sources++
	}
	if c.KeyStdin {
		sources++
	}
	if keyEnv != "" {
		sources++
	}
	if sources == 0 {
		return nil, "", usage("provide service account key with --key, --key=-, --key-stdin, or --key-env")
	}
	if sources > 1 {
		return nil, "", usage("provide exactly one service account key source")
	}

	switch {
	case c.KeyStdin || keyPath == "-":
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, "", fmt.Errorf("read service account key from stdin: %w", err)
		}

		return data, "stdin", nil
	case keyEnv != "":
		value, ok := os.LookupEnv(keyEnv)
		if !ok {
			return nil, "", usagef("environment variable %s is not set", keyEnv)
		}

		return []byte(value), "env:" + keyEnv, nil
	default:
		expanded, err := config.ExpandPath(keyPath)
		if err != nil {
			return nil, "", err
		}

		data, err := os.ReadFile(expanded) //nolint:gosec // user-provided path
		if err != nil {
			return nil, "", fmt.Errorf("read service account key: %w", err)
		}

		return data, expanded, nil
	}
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

	if err := dryRunAndConfirmDestructive(ctx, flags, "auth.service_account.unset", map[string]any{
		"email": email,
	}, fmt.Sprintf("remove stored service account for %s", email)); err != nil {
		return err
	}

	path, err := config.ServiceAccountPath(email)
	if err != nil {
		return err
	}

	removed, err := config.RemoveServiceAccountFiles(email)
	if err != nil {
		return fmt.Errorf("remove service account: %w", err)
	}

	return writeResult(ctx, u,
		kv("deleted", removed),
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

	path, err := config.ExistingServiceAccountPath(email)
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
					"hint":    fmt.Sprintf("configure with: gog auth service-account set %s --key <service-account.json>", email),
				})
			}
			return writeResult(ctx, u,
				kv("email", email),
				kv("path", path),
				kv("exists", false),
				kv("stored", false),
				kv("message", "no service account configured"),
				kv("hint", fmt.Sprintf("gog auth service-account set %s --key <service-account.json>", email)),
			)
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
	kvs := []resultKV{
		kv("email", email),
		kv("path", path),
		kv("exists", true),
		kv("stored", true),
	}
	if info.ClientEmail != "" {
		kvs = append(kvs, kv("client_email", info.ClientEmail))
	}
	if info.ClientID != "" {
		kvs = append(kvs, kv("client_id", info.ClientID))
	}
	return writeResult(ctx, u, kvs...)
}
