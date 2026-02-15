package cmd

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/idtoken"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var (
	newOIDCValidator    = idtoken.NewValidator
	listenAndServe      = func(srv *http.Server) error { return srv.ListenAndServe() }
	errNoHookConfigured = errors.New("no hook configured")
)

type GmailWatchCmd struct {
	Start  GmailWatchStartCmd  `cmd:"" name:"start" aliases:"begin" help:"Start Gmail watch for Pub/Sub"`
	Status GmailWatchStatusCmd `cmd:"" name:"status" aliases:"ls" help:"Show stored watch state"`
	Renew  GmailWatchRenewCmd  `cmd:"" name:"renew" aliases:"update" help:"Renew Gmail watch using stored config"`
	Stop   GmailWatchStopCmd   `cmd:"" name:"stop" aliases:"rm,delete" help:"Stop Gmail watch and clear stored state"`
	Serve  GmailWatchServeCmd  `cmd:"" name:"serve" help:"Run Pub/Sub push handler"`
}

type GmailWatchStartCmd struct {
	Topic       string   `name:"topic" help:"Pub/Sub topic (projects/.../topics/...)"`
	Labels      []string `name:"label" help:"Label IDs or names (repeatable, comma-separated)"`
	TTL         string   `name:"ttl" help:"Renew after duration (seconds or Go duration)"`
	HookURL     string   `name:"hook-url" help:"Webhook URL to forward messages"`
	HookToken   string   `name:"hook-token" help:"Webhook bearer token"`
	IncludeBody bool     `name:"include-body" help:"Include text/plain body in hook payload"`
	MaxBytes    int      `name:"max-bytes" help:"Max bytes of body to include" default:"20000"`
}

func (c *GmailWatchStartCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	if strings.TrimSpace(c.Topic) == "" {
		return usage("--topic is required")
	}
	ttl, err := parseDurationSeconds(c.TTL)
	if err != nil {
		return err
	}
	maxChanged := flagProvided(kctx, "max-bytes")
	hook, err := hookFromFlags(c.HookURL, c.HookToken, c.IncludeBody, c.MaxBytes, maxChanged, false)
	if err != nil {
		if errors.Is(err, errNoHookConfigured) {
			hook = nil
		} else {
			return err
		}
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.watch.start", map[string]any{
		"topic":   strings.TrimSpace(c.Topic),
		"labels":  c.Labels,
		"ttl_raw": strings.TrimSpace(c.TTL),
		"ttl":     ttl.String(),
		"hook":    hook,
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
	labelIDs, err := resolveLabelIDsWithService(svc, c.Labels)
	if err != nil {
		return err
	}

	resp, err := requestGmailWatch(ctx, svc, c.Topic, labelIDs)
	if err != nil {
		return err
	}
	state, err := buildWatchState(account, c.Topic, labelIDs, resp, ttl, hook)
	if err != nil {
		return err
	}

	store, err := newGmailWatchStore(account)
	if err != nil {
		return err
	}
	if err := store.Update(func(s *gmailWatchState) error {
		*s = state
		return nil
	}); err != nil {
		return err
	}

	return writeWatchState(ctx, state)
}

type GmailWatchStatusCmd struct{}

func (c *GmailWatchStatusCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	store, err := loadGmailWatchStore(account)
	if err != nil {
		return err
	}
	return writeWatchState(ctx, store.Get())
}

type GmailWatchRenewCmd struct {
	TTL string `name:"ttl" help:"Renew after duration (seconds or Go duration)"`
}

func (c *GmailWatchRenewCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	store, err := loadGmailWatchStore(account)
	if err != nil {
		return err
	}
	state := store.Get()
	if strings.TrimSpace(state.Topic) == "" {
		return errors.New("stored watch state missing topic")
	}

	ttl, err := parseDurationSeconds(c.TTL)
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "gmail.watch.renew", map[string]any{
		"topic":   strings.TrimSpace(state.Topic),
		"labels":  state.Labels,
		"ttl_raw": strings.TrimSpace(c.TTL),
		"ttl":     ttl.String(),
		"hook":    state.Hook,
	}); dryRunErr != nil {
		return dryRunErr
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}
	resp, err := requestGmailWatch(ctx, svc, state.Topic, state.Labels)
	if err != nil {
		return err
	}
	updated, err := buildWatchState(account, state.Topic, state.Labels, resp, ttl, state.Hook)
	if err != nil {
		return err
	}
	if ttl == 0 {
		updated.RenewAfterMs = state.RenewAfterMs
	}

	if err := store.Update(func(s *gmailWatchState) error {
		*s = updated
		return nil
	}); err != nil {
		return err
	}

	return writeWatchState(ctx, updated)
}

type GmailWatchStopCmd struct{}

func (c *GmailWatchStopCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	if confirmErr := confirmDestructive(ctx, flags, "stop gmail watch and clear stored state"); confirmErr != nil {
		return confirmErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}
	if stopErr := svc.Users.Stop("me").Do(); stopErr != nil {
		return stopErr
	}
	store, err := newGmailWatchStore(account)
	if err == nil && store.path != "" {
		_ = os.Remove(store.path)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"stopped": true})
	}
	u.Out().Printf("stopped\ttrue")
	return nil
}

type GmailWatchServeCmd struct {
	Bind          string `name:"bind" help:"Bind address" default:"127.0.0.1"`
	Port          int    `name:"port" help:"Listen port" default:"8788"`
	Path          string `name:"path" help:"Push handler path" default:"/gmail-pubsub"`
	Timezone      string `name:"timezone" short:"z" help:"Output timezone (IANA name, e.g. America/New_York, UTC). Default: local"`
	Local         bool   `name:"local" help:"Use local timezone (default behavior, useful to override --timezone)"`
	VerifyOIDC    bool   `name:"verify-oidc" help:"Verify Pub/Sub OIDC tokens"`
	OIDCEmail     string `name:"oidc-email" help:"Expected service account email"`
	OIDCAudience  string `name:"oidc-audience" help:"Expected OIDC audience"`
	SharedToken   string `name:"token" help:"Shared token for x-gog-token or ?token="`
	HookURL       string `name:"hook-url" help:"Webhook URL to forward messages"`
	HookToken     string `name:"hook-token" help:"Webhook bearer token"`
	IncludeBody   bool   `name:"include-body" help:"Include text/plain body in hook payload"`
	MaxBytes      int    `name:"max-bytes" help:"Max bytes of body to include" default:"20000"`
	ExcludeLabels string `name:"exclude-labels" help:"List of Gmail label IDs to exclude from hook payload (e.g. SPAM,TRASH,Label_123). Set to empty string to disable." default:"SPAM,TRASH"`
	SaveHook      bool   `name:"save-hook" help:"Persist hook settings to watch state"`
}

func (c *GmailWatchServeCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(c.Path, "/") {
		return usage("--path must start with '/'")
	}
	if c.Port <= 0 {
		return usage("--port must be > 0")
	}
	if !c.VerifyOIDC && c.SharedToken == "" && !isLoopbackHost(c.Bind) {
		return usage("--verify-oidc or --token required when binding non-loopback")
	}
	if c.OIDCEmail != "" && !c.VerifyOIDC {
		return usage("--oidc-email requires --verify-oidc")
	}
	if c.OIDCAudience != "" && !c.VerifyOIDC {
		return usage("--oidc-audience requires --verify-oidc")
	}

	loc, err := resolveOutputLocation(c.Timezone, c.Local)
	if err != nil {
		return err
	}

	store, err := loadGmailWatchStore(account)
	if err != nil {
		return err
	}
	state := store.Get()

	hookURL := c.HookURL
	hookToken := c.HookToken
	includeBody := c.IncludeBody
	maxBytes := c.MaxBytes

	if hookURL == "" && state.Hook != nil {
		hookURL = state.Hook.URL
		if !flagProvided(kctx, "hook-token") {
			hookToken = state.Hook.Token
		}
		if !flagProvided(kctx, "include-body") {
			includeBody = state.Hook.IncludeBody
		}
		if !flagProvided(kctx, "max-bytes") && state.Hook.MaxBytes > 0 {
			maxBytes = state.Hook.MaxBytes
		}
	}

	maxChanged := flagProvided(kctx, "max-bytes")
	hook, err := hookFromFlags(hookURL, hookToken, includeBody, maxBytes, maxChanged, true)
	if err != nil {
		if errors.Is(err, errNoHookConfigured) {
			hook = nil
		} else {
			return err
		}
	}
	if c.SaveHook && hook != nil {
		if updateErr := store.Update(func(s *gmailWatchState) error {
			s.Hook = hook
			s.UpdatedAtMs = time.Now().UnixMilli()
			return nil
		}); updateErr != nil {
			return updateErr
		}
	}

	validator := (*idtoken.Validator)(nil)
	if c.VerifyOIDC {
		validator, err = newOIDCValidator(ctx)
		if err != nil {
			return err
		}
	}

	cfg := gmailWatchServeConfig{
		Account:       account,
		Bind:          c.Bind,
		Port:          c.Port,
		Path:          c.Path,
		VerifyOIDC:    c.VerifyOIDC,
		OIDCEmail:     c.OIDCEmail,
		OIDCAudience:  c.OIDCAudience,
		SharedToken:   c.SharedToken,
		HookTimeout:   defaultHookRequestTimeoutSec * time.Second,
		HistoryMax:    defaultHistoryMaxResults,
		ResyncMax:     defaultHistoryResyncMax,
		AllowNoHook:   hook == nil,
		IncludeBody:   includeBody,
		MaxBodyBytes:  maxBytes,
		DateLocation:  loc,
		ExcludeLabels: splitCommaList(c.ExcludeLabels),
		VerboseOutput: flags.Verbose,
	}
	if hook != nil {
		cfg.HookURL = hook.URL
		cfg.HookToken = hook.Token
		cfg.IncludeBody = hook.IncludeBody
		cfg.MaxBodyBytes = hook.MaxBytes
	}

	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultHookMaxBytes
	}

	hookClient := &http.Client{Timeout: cfg.HookTimeout}
	server := &gmailWatchServer{
		cfg:             cfg,
		store:           store,
		validator:       validator,
		newService:      newGmailService,
		hookClient:      hookClient,
		excludeLabelIDs: stringSet(cfg.ExcludeLabels),
		logf:            u.Err().Printf,
		warnf:           u.Err().Printf,
	}

	addr := net.JoinHostPort(c.Bind, strconv.Itoa(c.Port))
	u.Err().Printf("watch: listening on %s%s", addr, c.Path)

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return listenAndServe(httpServer)
}

func writeWatchState(ctx context.Context, state gmailWatchState) error {
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"watch": state})
	}
	u := ui.FromContext(ctx)
	u.Out().Printf("account\t%s", state.Account)
	u.Out().Printf("topic\t%s", state.Topic)
	if len(state.Labels) > 0 {
		u.Out().Printf("labels\t%s", strings.Join(state.Labels, ","))
	}
	u.Out().Printf("history_id\t%s", state.HistoryID)
	if state.ExpirationMs > 0 {
		u.Out().Printf("expiration\t%s", formatUnixMillis(state.ExpirationMs))
	}
	if state.ProviderExpirationMs > 0 {
		u.Out().Printf("provider_expiration\t%s", formatUnixMillis(state.ProviderExpirationMs))
	}
	if state.RenewAfterMs > 0 {
		u.Out().Printf("renew_after\t%s", formatUnixMillis(state.RenewAfterMs))
	}
	if state.UpdatedAtMs > 0 {
		u.Out().Printf("updated_at\t%s", formatUnixMillis(state.UpdatedAtMs))
	}
	if state.Hook != nil {
		u.Out().Printf("hook_url\t%s", state.Hook.URL)
		if state.Hook.IncludeBody {
			u.Out().Printf("hook_include_body\ttrue")
		}
		if state.Hook.MaxBytes > 0 {
			u.Out().Printf("hook_max_bytes\t%d", state.Hook.MaxBytes)
		}
		if state.Hook.Token != "" {
			u.Out().Printf("hook_token\t%s", state.Hook.Token)
		}
	}
	if state.LastDeliveryStatus != "" {
		u.Out().Printf("last_delivery_status\t%s", state.LastDeliveryStatus)
	}
	if state.LastDeliveryAtMs > 0 {
		u.Out().Printf("last_delivery_at\t%s", formatUnixMillis(state.LastDeliveryAtMs))
	}
	if state.LastDeliveryStatusNote != "" {
		u.Out().Printf("last_delivery_note\t%s", state.LastDeliveryStatusNote)
	}
	if state.LastPushMessageID != "" {
		u.Out().Printf("last_push_message_id\t%s", state.LastPushMessageID)
	}
	return nil
}

func buildWatchState(account, topic string, labels []string, resp *gmail.WatchResponse, ttl time.Duration, hook *gmailWatchHook) (gmailWatchState, error) {
	if resp == nil {
		return gmailWatchState{}, errors.New("watch response missing")
	}
	historyID := formatHistoryID(resp.HistoryId)
	if historyID == "" {
		return gmailWatchState{}, errors.New("watch response missing historyId")
	}
	now := time.Now()
	state := gmailWatchState{
		Account:              account,
		Topic:                topic,
		Labels:               labels,
		HistoryID:            historyID,
		ExpirationMs:         resp.Expiration,
		ProviderExpirationMs: resp.Expiration,
		UpdatedAtMs:          now.UnixMilli(),
		Hook:                 hook,
	}
	if ttl > 0 {
		state.RenewAfterMs = now.Add(ttl).UnixMilli()
	}
	return state, nil
}

func requestGmailWatch(ctx context.Context, svc *gmail.Service, topic string, labelIDs []string) (*gmail.WatchResponse, error) {
	req := &gmail.WatchRequest{TopicName: topic}
	if len(labelIDs) > 0 {
		req.LabelIds = labelIDs
	}
	return svc.Users.Watch("me", req).Context(ctx).Do()
}

func hookFromFlags(url, token string, includeBody bool, maxBytes int, maxBytesChanged bool, allowNoHook bool) (*gmailWatchHook, error) {
	if strings.TrimSpace(url) == "" {
		if token != "" {
			return nil, usage("--hook-url required when using --hook-token")
		}
		if !allowNoHook && (includeBody || maxBytesChanged) {
			return nil, usage("--hook-url required when setting hook options")
		}
		return nil, errNoHookConfigured
	}
	if maxBytes <= 0 {
		if includeBody {
			maxBytes = defaultHookMaxBytes
		} else if maxBytesChanged {
			return nil, usage("--max-bytes must be > 0")
		}
	}
	return &gmailWatchHook{
		URL:         url,
		Token:       token,
		IncludeBody: includeBody,
		MaxBytes:    maxBytes,
	}, nil
}

func isLoopbackHost(host string) bool {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return true
	}
	if strings.EqualFold(trimmed, "localhost") {
		return true
	}
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
