package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/secrets"
	"github.com/steipete/gogcli/internal/ui"
)

type AuthDoctorCmd struct {
	Check   bool          `name:"check" help:"Verify refresh tokens by exchanging for access tokens"`
	Timeout time.Duration `name:"timeout" help:"Per-token check timeout" default:"15s"`
}

type authDoctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

const (
	doctorOK    = "ok"
	doctorWarn  = "warn"
	doctorError = literalError
)

func (c *AuthDoctorCmd) Run(ctx context.Context, _ *RootFlags) error {
	u := ui.FromContext(ctx)
	checks := make([]authDoctorCheck, 0)

	add := func(name, status, detail, hint string) {
		checks = append(checks, authDoctorCheck{
			Name:   name,
			Status: status,
			Detail: detail,
			Hint:   hint,
		})
	}

	configPath, pathErr := config.ConfigPath()
	if pathErr != nil {
		add("config.path", doctorError, pathErr.Error(), "")
	} else {
		exists, existsErr := config.ConfigExists()
		switch {
		case existsErr != nil:
			add("config.path", doctorError, existsErr.Error(), "")
		case exists:
			add("config.path", doctorOK, configPath, "")
		default:
			add("config.path", doctorWarn, configPath+" (missing)", "run `gog auth credentials <credentials.json>` or another config-writing auth command")
		}
	}

	backendInfo, backendErr := secrets.ResolveKeyringBackendInfo()
	if backendErr != nil {
		add("keyring.backend", doctorError, backendErr.Error(), "")
	} else {
		add("keyring.backend", doctorOK, backendInfo.Value+" (source: "+backendInfo.Source+")", "")
		addKeyringEnvChecks(add, backendInfo)
	}

	store, storeErr := openSecretsStore()
	if storeErr != nil {
		status, hint := classifyAuthDoctorError(storeErr)
		add("keyring.open", status, storeErr.Error(), hint)
		return writeAuthDoctorResult(ctx, u, checks)
	}
	add("keyring.open", doctorOK, "opened", "")

	keys, keysErr := store.Keys()
	if keysErr != nil {
		status, hint := classifyAuthDoctorError(keysErr)
		add("keyring.keys", status, keysErr.Error(), hint)
		return writeAuthDoctorResult(ctx, u, checks)
	}

	tokens := make([]secrets.Token, 0)
	tokenKeys := 0
	seenTokens := make(map[string]struct{})
	for _, key := range keys {
		client, email, ok := secrets.ParseTokenKey(key)
		if !ok {
			continue
		}
		tokenID := client + "\n" + email
		if _, seen := seenTokens[tokenID]; seen {
			continue
		}
		seenTokens[tokenID] = struct{}{}
		tokenKeys++
		tok, tokErr := store.GetToken(client, email)
		if tokErr != nil {
			status, hint := classifyAuthDoctorError(tokErr)
			add(authDoctorTokenCheckName("token", client, email), status, tokErr.Error(), hint)
			continue
		}
		tokens = append(tokens, tok)
	}

	if tokenKeys == 0 {
		add("tokens", doctorWarn, "no OAuth tokens stored", "run `gog auth add <email>`")
	} else {
		add("tokens", doctorOK, pluralizeCount(len(tokens), "readable OAuth token")+" of "+pluralizeCount(tokenKeys, "stored token account"), "")
	}

	if c.Check {
		for _, tok := range tokens {
			err := checkRefreshToken(ctx, tok.Client, tok.RefreshToken, tok.Scopes, c.Timeout)
			if err == nil {
				add(authDoctorTokenCheckName("refresh", tok.Client, tok.Email), doctorOK, "refresh token exchange succeeded", "")
				continue
			}
			_, hint := classifyAuthDoctorError(err)
			add(authDoctorTokenCheckName("refresh", tok.Client, tok.Email), doctorError, err.Error(), hint)
		}
	}

	return writeAuthDoctorResult(ctx, u, checks)
}

func authDoctorTokenCheckName(prefix string, client string, email string) string {
	client = strings.TrimSpace(client)
	if client == "" {
		client = config.DefaultClientName
	}
	return prefix + "." + client + "." + email
}

func addKeyringEnvChecks(add func(string, string, string, string), backendInfo secrets.KeyringBackendInfo) {
	cfg, cfgErr := config.ReadConfig()
	if cfgErr != nil {
		add("keyring.config", doctorError, cfgErr.Error(), "")
	}

	envBackend := strings.TrimSpace(os.Getenv("GOG_KEYRING_BACKEND"))
	if envBackend != "" && cfgErr == nil && strings.TrimSpace(cfg.KeyringBackend) != "" && !strings.EqualFold(envBackend, cfg.KeyringBackend) {
		add("keyring.backend_override", doctorWarn, "GOG_KEYRING_BACKEND overrides config.json keyring_backend", "make env and config agree before debugging stored tokens")
	}

	keyringDir, dirErr := config.KeyringDir()
	if dirErr != nil {
		add("keyring.dir", doctorError, dirErr.Error(), "")
	} else {
		add("keyring.dir", doctorOK, keyringDir, "")
	}

	password, passwordSet := os.LookupEnv("GOG_KEYRING_PASSWORD")
	likelyFile := backendInfo.Value == strFile || (runtime.GOOS == "linux" && backendInfo.Value == "auto" && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "")
	if !likelyFile {
		return
	}

	switch {
	case passwordSet && password == "":
		add("keyring.password", doctorWarn, "GOG_KEYRING_PASSWORD is set to an empty string", "empty is valid but easy to set accidentally; keep it identical in every shell/service")
	case passwordSet:
		add("keyring.password", doctorOK, "GOG_KEYRING_PASSWORD is set", "keep this value identical across shell, service, and agent configs")
	case !term.IsTerminal(int(os.Stdin.Fd())): //nolint:gosec // os file descriptor fits int on supported targets
		add("keyring.password", doctorError, "file keyring selected but GOG_KEYRING_PASSWORD is not set in a non-interactive process", "set GOG_KEYRING_PASSWORD or switch to a system keyring")
	default:
		add("keyring.password", doctorWarn, "file keyring selected and GOG_KEYRING_PASSWORD is not set", "interactive prompts work locally, but CI/ssh/agents need GOG_KEYRING_PASSWORD")
	}
}

func writeAuthDoctorResult(ctx context.Context, u *ui.UI, checks []authDoctorCheck) error {
	status := authDoctorStatus(checks)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"status": status,
			"checks": checks,
		})
	}
	if u == nil {
		return nil
	}
	for _, check := range checks {
		u.Out().Printf("%s\t%s\t%s", check.Status, check.Name, check.Detail)
		if check.Hint != "" {
			u.Out().Printf("hint\t%s\t%s", check.Name, check.Hint)
		}
	}
	u.Out().Printf("status\t%s", status)
	return nil
}

func authDoctorStatus(checks []authDoctorCheck) string {
	status := doctorOK
	for _, check := range checks {
		switch check.Status {
		case doctorError:
			return doctorError
		case doctorWarn:
			status = doctorWarn
		}
	}
	return status
}

func classifyAuthDoctorError(err error) (status string, hint string) {
	if err == nil {
		return doctorOK, ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "aes.keyunwrap") || strings.Contains(msg, "integrity check failed"):
		return doctorError, "file keyring password mismatch or corrupted entry; make every GOG_KEYRING_PASSWORD definition match, then re-run `gog auth doctor --check`"
	case strings.Contains(msg, "invalid_rapt"):
		return doctorError, "Google requires recent Workspace reauthentication; for automation prefer Workspace service-account domain-wide delegation, or re-run `gog auth add <email> --force-consent`"
	case strings.Contains(msg, "invalid_grant"):
		return doctorError, "refresh token was revoked, expired, or blocked by OAuth app policy; re-run `gog auth add <email> --force-consent` and verify the OAuth consent app is published for long-lived use"
	case strings.Contains(msg, "no tty") || strings.Contains(msg, "gog_keyring_password"):
		return doctorError, "file keyring needs GOG_KEYRING_PASSWORD in non-interactive shells, services, CI, and agents"
	case errors.Is(err, context.DeadlineExceeded):
		return doctorError, "keyring or token check timed out; try again from an interactive terminal or switch keyring backend"
	default:
		return doctorError, ""
	}
}

func pluralizeCount(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %ss", n, singular)
}
