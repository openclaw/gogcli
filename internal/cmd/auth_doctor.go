package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

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

	configStore, configErr := commandConfigStore(ctx)
	if configErr != nil {
		add("config.path", doctorError, configErr.Error(), "")
	} else {
		configPath := configStore.Path()
		exists, existsErr := configStore.Exists()
		switch {
		case existsErr != nil:
			add("config.path", doctorError, existsErr.Error(), "")
		case exists:
			add("config.path", doctorOK, configPath, "")
		default:
			add("config.path", doctorWarn, configPath+" (missing)", "run `gog auth credentials <credentials.json>` or another config-writing auth command")
		}
	}

	backendInfo, backendErr := resolveKeyringBackendInfo(ctx)
	if backendErr != nil {
		add("keyring.backend", doctorError, backendErr.Error(), "")
	} else {
		add("keyring.backend", doctorOK, backendInfo.Value+" (source: "+backendInfo.Source+")", "")
		addKeyringEnvChecks(ctx, add, backendInfo)
	}

	store, storeErr := openAuthSecretsStore(ctx)
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
			err := checkAuthRefreshToken(ctx, tok.Client, tok.RefreshToken, tok.Scopes, c.Timeout)
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

func addKeyringEnvChecks(ctx context.Context, add func(string, string, string, string), backendInfo secrets.KeyringBackendInfo) {
	store, storeErr := commandConfigStore(ctx)
	if storeErr != nil {
		add("keyring.config", doctorError, storeErr.Error(), "")
	}

	cfg, cfgErr := config.File{}, storeErr
	if storeErr == nil {
		cfg, cfgErr = store.Read()
	}
	if cfgErr != nil {
		add("keyring.config", doctorError, cfgErr.Error(), "")
	}

	envBackend := strings.TrimSpace(os.Getenv("GOG_KEYRING_BACKEND"))
	if envBackend != "" && cfgErr == nil && strings.TrimSpace(cfg.KeyringBackend) != "" && !strings.EqualFold(envBackend, cfg.KeyringBackend) {
		add("keyring.backend_override", doctorWarn, "GOG_KEYRING_BACKEND overrides config.json keyring_backend", "make env and config agree before debugging stored tokens")
	}

	if backendInfo.Value == secrets.KeyringBackendOnePassword {
		addOnePasswordEnvChecks(add, cfg)
		return
	}

	layout, layoutErr := commandLayout(ctx, config.PathKindConfig, config.PathKindData)
	keyringDir, dirErr := "", layoutErr
	if layoutErr == nil {
		keyringDir = layout.KeyringDir()
	}
	if dirErr != nil {
		add("keyring.dir", doctorError, dirErr.Error(), "")
	} else {
		add("keyring.dir", doctorOK, keyringDir, "")
	}

	password, passwordSet := os.LookupEnv("GOG_KEYRING_PASSWORD")
	likelyFile := backendInfo.Value == strFile || (runtime.GOOS == "linux" && backendInfo.Value == literalAuto && os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "")
	if !likelyFile {
		return
	}

	switch {
	case passwordSet && password == "":
		add("keyring.password", doctorWarn, "GOG_KEYRING_PASSWORD is set to an empty string", "empty is valid but easy to set accidentally; keep it identical in every shell/service")
	case passwordSet:
		add("keyring.password", doctorOK, "GOG_KEYRING_PASSWORD is set", "keep this value identical across shell, service, and agent configs")
	case !stdinIsTerminal(ctx):
		add("keyring.password", doctorError, "file keyring selected but GOG_KEYRING_PASSWORD is not set in a non-interactive process", "set GOG_KEYRING_PASSWORD or switch to a system keyring")
	default:
		add("keyring.password", doctorWarn, "file keyring selected and GOG_KEYRING_PASSWORD is not set", "interactive prompts work locally, but CI/ssh/agents need GOG_KEYRING_PASSWORD")
	}
}

func addOnePasswordEnvChecks(add func(string, string, string, string), cfg config.File) {
	vault, vaultSource := onePasswordConfigValue(secrets.OnePasswordVaultEnv, config.KeyOnePasswordVault.String(), cfg.OnePasswordVault)
	if vault == "" {
		add("keyring.1password.vault", doctorError, secrets.OnePasswordVaultEnv+" and config onepassword_vault are not set", "set the 1Password vault ID that stores gog items")
	} else {
		add("keyring.1password.vault", doctorOK, vaultSource+" is set", "")
	}

	authRaw, _ := onePasswordConfigValue(secrets.OnePasswordAuthEnv, config.KeyOnePasswordAuth.String(), cfg.OnePasswordAuth)
	authMode := secrets.NormalizeOnePasswordAuthMode(authRaw)
	account, accountSource := onePasswordConfigValue(secrets.OnePasswordAccountEnv, config.KeyOnePasswordAccount.String(), cfg.OnePasswordAccount)
	token := strings.TrimSpace(os.Getenv(secrets.OnePasswordServiceAccountEnv))

	switch authMode {
	case "", "auto":
		addOnePasswordAutoAuthCheck(add, account, accountSource, token)
	case "desktop":
		addOnePasswordDesktopAuthCheck(add, account, accountSource)
	case "service-account":
		addOnePasswordServiceAccountAuthCheck(add, token)
	default:
		add("keyring.1password.auth", doctorError, "invalid 1Password auth mode "+authRaw, "use auto, desktop, or service-account")
	}

	if raw, source := onePasswordConfigValue(secrets.OnePasswordOperationTimeoutEnv, config.KeyOnePasswordTimeout.String(), cfg.OnePasswordTimeout); raw != "" {
		if timeout, err := time.ParseDuration(raw); err != nil || timeout <= 0 {
			add("keyring.1password.timeout", doctorError, "invalid "+source, "use a positive duration such as 10s")
		} else {
			add("keyring.1password.timeout", doctorOK, source+"="+raw, "")
		}
	}
}

func onePasswordConfigValue(envName, key string, cfgValue string) (string, string) {
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value, envName
	}
	if value := strings.TrimSpace(cfgValue); value != "" {
		return value, "config " + key
	}
	return "", ""
}

func addOnePasswordAutoAuthCheck(add func(string, string, string, string), account string, accountSource string, token string) {
	switch {
	case account != "":
		add("keyring.1password.auth", doctorOK, "desktop app auth via "+accountSource, "keep the 1Password app running, unlocked, and configured to integrate with other apps")
	case token != "":
		add("keyring.1password.auth", doctorOK, "service-account auth via "+secrets.OnePasswordServiceAccountEnv, "")
	default:
		add("keyring.1password.auth", doctorError, "no 1Password auth source is set", "set "+secrets.OnePasswordAccountEnv+" or config onepassword_account for desktop app auth, or "+secrets.OnePasswordServiceAccountEnv+" for service-account auth")
	}
}

func addOnePasswordDesktopAuthCheck(add func(string, string, string, string), account string, accountSource string) {
	if account == "" {
		add("keyring.1password.account", doctorError, secrets.OnePasswordAccountEnv+" and config onepassword_account are not set", "set the 1Password account name or UUID shown in the desktop app")
	} else {
		add("keyring.1password.account", doctorOK, accountSource+" is set", "keep the 1Password app running, unlocked, and configured to integrate with other apps")
	}
}

func addOnePasswordServiceAccountAuthCheck(add func(string, string, string, string), token string) {
	if token == "" {
		add("keyring.1password.token", doctorError, secrets.OnePasswordServiceAccountEnv+" is not set", "set a 1Password service account token with read/write access to the vault")
	} else {
		add("keyring.1password.token", doctorOK, secrets.OnePasswordServiceAccountEnv+" is set", "")
	}
}

func writeAuthDoctorResult(ctx context.Context, u *ui.UI, checks []authDoctorCheck) error {
	status := authDoctorStatus(checks)
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"status": status,
			"checks": checks,
		})
	}
	if u == nil {
		return nil
	}
	for _, check := range checks {
		u.Out().Linef("%s\t%s\t%s", check.Status, check.Name, check.Detail)
		if check.Hint != "" {
			u.Out().Linef("hint\t%s\t%s", check.Name, check.Hint)
		}
	}
	u.Out().Linef("status\t%s", status)
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
