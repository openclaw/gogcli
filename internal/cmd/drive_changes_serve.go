package cmd

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	defaultDriveChangesChannelTTL          = 24 * time.Hour
	defaultDriveChangesNotificationTimeout = 5 * time.Minute
	defaultDriveChangesReadTimeout         = 10 * time.Second
	maxDriveChangesChannelTTL              = 7 * 24 * time.Hour
	driveChangesRenewRetry                 = time.Minute
)

var errDriveChangesPreviousCleanupPending = errors.New("previous Drive changes channel cleanup is pending")

type DriveChangesServeCmd struct {
	Listen              string        `name:"listen" help:"Listen address" default:"127.0.0.1:8443"`
	Path                string        `name:"path" help:"Notification handler path" default:"/drive-changes"`
	Cert                string        `name:"cert" help:"TLS certificate path; pair with --key (omit behind an HTTPS reverse proxy)"`
	Key                 string        `name:"key" help:"TLS private key path; pair with --cert"`
	ChannelToken        string        `name:"channel-token" help:"Expected X-Goog-Channel-Token value"`
	ChannelTokenFile    string        `name:"channel-token-file" type:"path" help:"Read the expected channel token from a file"`
	StateFile           string        `name:"state-file" required:"" help:"JSON file that stores the current Drive page token and channel state"`
	Token               string        `name:"token" help:"Initial Drive page token when creating a new state file"`
	OnChange            string        `name:"on-change" help:"Trusted local shell command run for each non-empty change batch; event JSON is provided on stdin"`
	FilterFile          string        `name:"filter-file" help:"Only invoke the hook for changes to this file ID"`
	DriveID             string        `name:"drive" aliases:"drive-id" help:"Shared drive ID for a shared-drive change log"`
	Max                 int64         `name:"max" aliases:"limit" help:"Max changes per API page" default:"100"`
	IncludeRemoved      bool          `name:"include-removed" help:"Include removed changes" default:"true" negatable:"_"`
	AutoRenew           bool          `name:"auto-renew" help:"Create and renew the Drive notification channel"`
	WebhookURL          string        `name:"webhook-url" help:"Public HTTPS callback URL used by --auto-renew"`
	ChannelTTL          time.Duration `name:"channel-ttl" help:"Requested channel lifetime" default:"24h"`
	RenewBefore         time.Duration `name:"renew-before" help:"Renew this long before channel expiration" default:"10m"`
	NotificationTimeout time.Duration `name:"notification-timeout" help:"Maximum time for one callback, including Drive reads and the hook" default:"5m"`
}

type driveChangesServeChannelState struct {
	ID          string `json:"id"`
	ResourceID  string `json:"resource_id"`
	ResourceURI string `json:"resource_uri,omitempty"`
	Expiration  int64  `json:"expiration_ms"`
	WebhookURL  string `json:"webhook_url,omitempty"`
	TokenHash   string `json:"token_sha256,omitempty"`
}

type driveChangesServeState struct {
	Version            int                            `json:"version"`
	Kind               string                         `json:"kind,omitempty"`
	PageToken          string                         `json:"page_token"`
	DriveID            string                         `json:"drive_id,omitempty"`
	Channel            *driveChangesServeChannelState `json:"channel,omitempty"`
	PreviousChannel    *driveChangesServeChannelState `json:"previous_channel,omitempty"`
	LastMessageNumbers map[string]uint64              `json:"last_message_numbers,omitempty"`
	UpdatedAt          string                         `json:"updated_at"`
}

type driveChangesServeRuntime struct {
	now     func() time.Time
	runHook func(context.Context, string, any) error
	wait    func(context.Context, time.Duration) error
	listen  func(context.Context, string, string) (net.Listener, error)
}

func defaultDriveChangesServeRuntime() driveChangesServeRuntime {
	var listenConfig net.ListenConfig
	return driveChangesServeRuntime{
		now:     time.Now,
		runHook: runJSONShellHook,
		wait:    waitForPollInterval,
		listen:  listenConfig.Listen,
	}
}

func (r driveChangesServeRuntime) withDefaults() driveChangesServeRuntime {
	defaults := defaultDriveChangesServeRuntime()
	if r.now == nil {
		r.now = defaults.now
	}
	if r.runHook == nil {
		r.runHook = defaults.runHook
	}
	if r.wait == nil {
		r.wait = defaults.wait
	}
	if r.listen == nil {
		r.listen = defaults.listen
	}
	return r
}

type driveChangesServer struct {
	mu                  sync.Mutex
	notificationOnce    sync.Once
	notificationGate    chan struct{}
	renewMu             sync.Mutex
	runDone             <-chan struct{}
	pendingChannel      string
	statePath           string
	state               driveChangesServeState
	service             *drive.Service
	path                string
	channelToken        string
	onChange            string
	filterFile          string
	max                 int64
	includeRemoved      bool
	autoRenew           bool
	webhookURL          string
	channelTTL          time.Duration
	renewBefore         time.Duration
	notificationTimeout time.Duration
	runtime             driveChangesServeRuntime
	logf                func(string, ...any)
	warnf               func(string, ...any)
}

func (c *DriveChangesServeCmd) Run(ctx context.Context, flags *RootFlags) error {
	serveCtx, stop := pollSignalContext(ctx)
	defer stop()
	return c.run(serveCtx, flags, defaultDriveChangesServeRuntime())
}

func (c *DriveChangesServeCmd) run(ctx context.Context, flags *RootFlags, runtime driveChangesServeRuntime) error {
	runtime = runtime.withDefaults()
	u := ui.FromContext(ctx)
	statePath, err := expandPollStatePath(c.StateFile)
	if err != nil {
		return err
	}
	channelToken, err := c.resolveChannelToken()
	if err != nil {
		return err
	}
	if validationErr := c.validate(channelToken); validationErr != nil {
		return validationErr
	}

	driveID := strings.TrimSpace(c.DriveID)
	filterFile := normalizeGoogleID(strings.TrimSpace(c.FilterFile))
	if dryRunErr := dryRunExit(ctx, flags, "drive.changes.serve", map[string]any{
		"listen":               strings.TrimSpace(c.Listen),
		"path":                 strings.TrimSpace(c.Path),
		"tls":                  strings.TrimSpace(c.Cert) != "",
		"state_file":           statePath,
		"initial_token":        strings.TrimSpace(c.Token) != "",
		"drive_id":             driveID,
		"filter_file":          filterFile,
		"max":                  c.Max,
		"include_removed":      c.IncludeRemoved,
		"hook_configured":      strings.TrimSpace(c.OnChange) != "",
		"auto_renew":           c.AutoRenew,
		"webhook_url":          strings.TrimSpace(c.WebhookURL),
		"channel_ttl":          c.ChannelTTL.String(),
		"renew_before":         c.RenewBefore.String(),
		"notification_timeout": c.NotificationTimeout.String(),
	}); dryRunErr != nil {
		return dryRunErr
	}

	var tlsConfig *tls.Config
	if strings.TrimSpace(c.Cert) != "" {
		certPath, keyPath, pathErr := expandDriveChangesTLSPaths(c.Cert, c.Key)
		if pathErr != nil {
			return pathErr
		}
		certificate, certErr := tls.LoadX509KeyPair(certPath, keyPath)
		if certErr != nil {
			return fmt.Errorf("load TLS certificate: %w", certErr)
		}
		tlsConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{certificate},
		}
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	state, err := initializeDriveChangesServeState(
		ctx,
		svc,
		statePath,
		strings.TrimSpace(c.Token),
		driveID,
		runtime.now().UTC(),
	)
	if err != nil {
		return err
	}
	listener, err := runtime.listen(ctx, "tcp", strings.TrimSpace(c.Listen))
	if err != nil {
		return fmt.Errorf("listen on %s: %w", c.Listen, err)
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}

	server := &driveChangesServer{
		runDone:             ctx.Done(),
		statePath:           statePath,
		state:               state,
		service:             svc,
		path:                strings.TrimSpace(c.Path),
		channelToken:        channelToken,
		onChange:            strings.TrimSpace(c.OnChange),
		filterFile:          filterFile,
		max:                 c.Max,
		includeRemoved:      c.IncludeRemoved,
		autoRenew:           c.AutoRenew,
		webhookURL:          strings.TrimSpace(c.WebhookURL),
		channelTTL:          c.ChannelTTL,
		renewBefore:         c.RenewBefore,
		notificationTimeout: c.NotificationTimeout,
		runtime:             runtime,
		logf:                u.Err().Linef,
		warnf:               u.Err().Linef,
	}
	httpServer := &http.Server{
		Handler:           server,
		ReadTimeout:       defaultDriveChangesReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- httpServer.Serve(listener)
	}()

	scheme := driveChangesServerSchemeHTTP
	if tlsConfig != nil {
		scheme = driveChangesWebhookSchemeHTTPS
	}
	u.Err().Linef("drive changes serve: listening on %s://%s%s", scheme, listener.Addr(), server.path)

	if c.AutoRenew {
		nextRenewal, renewErr := server.ensureChannel(ctx)
		if renewErr != nil {
			if !errors.Is(renewErr, errDriveChangesPreviousCleanupPending) {
				_ = shutdownHTTPServer(ctx, httpServer)
				return renewErr
			}
			server.warnf("drive changes serve: channel cleanup pending; retrying renewal in %s", driveChangesRenewRetry)
			nextRenewal = driveChangesRenewRetry
		}
		go server.runRenewLoop(ctx, nextRenewal)
	}

	select {
	case serveErr := <-serveErrors:
		if errors.Is(serveErr, http.ErrServerClosed) {
			return nil
		}
		return serveErr
	case <-ctx.Done():
		if shutdownErr := shutdownHTTPServer(ctx, httpServer); shutdownErr != nil {
			return shutdownErr
		}
		return ctx.Err()
	}
}

func (c *DriveChangesServeCmd) resolveChannelToken() (string, error) {
	direct := strings.TrimSpace(c.ChannelToken)
	tokenFile := strings.TrimSpace(c.ChannelTokenFile)
	if direct != "" && tokenFile != "" {
		return "", usage("provide only one of --channel-token or --channel-token-file")
	}
	if tokenFile != "" {
		path, err := config.ExpandPath(tokenFile)
		if err != nil {
			return "", fmt.Errorf("expand --channel-token-file: %w", err)
		}
		raw, err := os.ReadFile(path) //nolint:gosec // explicit operator-provided secret file.
		if err != nil {
			return "", fmt.Errorf("read --channel-token-file: %w", err)
		}
		direct = strings.TrimSpace(string(raw))
	}
	if direct == "" {
		direct = strings.TrimSpace(os.Getenv("GOG_DRIVE_CHANNEL_TOKEN"))
	}
	if direct == "" {
		return "", usage("provide --channel-token, --channel-token-file, or GOG_DRIVE_CHANNEL_TOKEN")
	}
	return direct, nil
}

func expandDriveChangesTLSPaths(cert string, key string) (string, string, error) {
	certPath, err := config.ExpandPath(strings.TrimSpace(cert))
	if err != nil {
		return "", "", fmt.Errorf("expand --cert: %w", err)
	}
	keyPath, err := config.ExpandPath(strings.TrimSpace(key))
	if err != nil {
		return "", "", fmt.Errorf("expand --key: %w", err)
	}
	return certPath, keyPath, nil
}

func (c *DriveChangesServeCmd) validate(channelToken string) error {
	if strings.TrimSpace(c.Listen) == "" {
		return usage("missing --listen")
	}
	if path := strings.TrimSpace(c.Path); path == "" || !strings.HasPrefix(path, "/") {
		return usage("--path must start with '/'")
	}
	if (strings.TrimSpace(c.Cert) == "") != (strings.TrimSpace(c.Key) == "") {
		return usage("--cert and --key must be provided together")
	}
	if len(channelToken) > 256 {
		return usage("--channel-token must be at most 256 bytes")
	}
	if c.Max <= 0 {
		return usage("--max must be greater than zero")
	}
	if c.NotificationTimeout <= 0 {
		return usage("--notification-timeout must be greater than zero")
	}
	webhookURL := strings.TrimSpace(c.WebhookURL)
	if c.AutoRenew {
		if webhookURL == "" {
			return usage("--webhook-url is required with --auto-renew")
		}
		if err := validateDriveChangesWebhookURL(webhookURL); err != nil {
			return err
		}
		if c.ChannelTTL <= 0 || c.ChannelTTL > maxDriveChangesChannelTTL {
			return usage("--channel-ttl must be greater than zero and at most 168h")
		}
		if c.RenewBefore <= 0 || c.RenewBefore >= c.ChannelTTL {
			return usage("--renew-before must be greater than zero and less than --channel-ttl")
		}
	} else if webhookURL != "" {
		return usage("--webhook-url requires --auto-renew")
	}
	return nil
}

func initializeDriveChangesServeState(
	ctx context.Context,
	svc *drive.Service,
	path string,
	initialToken string,
	driveID string,
	now time.Time,
) (driveChangesServeState, error) {
	state, exists, err := readDriveChangesServeState(path)
	if err != nil {
		return driveChangesServeState{}, err
	}
	if exists {
		if initialToken != "" && initialToken != state.PageToken {
			return driveChangesServeState{}, usage("--token does not match the persisted page token")
		}
		if driveID != "" && driveID != state.DriveID {
			return driveChangesServeState{}, usagef("serve state drive_id %q does not match --drive %q", state.DriveID, driveID)
		}
		return state, nil
	}
	if initialToken == "" {
		initialToken, err = getDriveChangesStartToken(ctx, svc, driveID)
		if err != nil {
			return driveChangesServeState{}, err
		}
	}
	state = driveChangesServeState{
		Version:   pollStateVersion,
		Kind:      driveChangesServeStateKind,
		PageToken: initialToken,
		DriveID:   driveID,
		UpdatedAt: now.Format(time.RFC3339Nano),
	}
	if err := writePollState(path, state); err != nil {
		return driveChangesServeState{}, err
	}
	return state, nil
}

func readDriveChangesServeState(path string) (driveChangesServeState, bool, error) {
	var state driveChangesServeState
	exists, err := readPollState(path, &state)
	if err != nil || !exists {
		return state, exists, err
	}
	if state.Version != pollStateVersion {
		return driveChangesServeState{}, false, fmt.Errorf("unsupported drive changes serve state version %d", state.Version)
	}
	switch state.Kind {
	case "", driveChangesServeStateKind:
		state.Kind = driveChangesServeStateKind
	case driveChangesPollStateKind:
		return driveChangesServeState{}, false, errors.New("state file belongs to drive changes poll; use a separate --state-file")
	default:
		return driveChangesServeState{}, false, fmt.Errorf("unsupported drive changes serve state kind %q", state.Kind)
	}
	state.PageToken = strings.TrimSpace(state.PageToken)
	state.DriveID = strings.TrimSpace(state.DriveID)
	if state.PageToken == "" {
		return driveChangesServeState{}, false, fmt.Errorf("drive changes serve state has empty page_token")
	}
	normalizeDriveChangesServeChannel(state.Channel)
	normalizeDriveChangesServeChannel(state.PreviousChannel)
	return state, true, nil
}

func normalizeDriveChangesServeChannel(channel *driveChangesServeChannelState) {
	if channel == nil {
		return
	}
	channel.ID = strings.TrimSpace(channel.ID)
	channel.ResourceID = strings.TrimSpace(channel.ResourceID)
	channel.ResourceURI = strings.TrimSpace(channel.ResourceURI)
	channel.WebhookURL = strings.TrimSpace(channel.WebhookURL)
	channel.TokenHash = strings.TrimSpace(channel.TokenHash)
}

func cloneDriveChangesServeState(state driveChangesServeState) driveChangesServeState {
	cloned := state
	if state.Channel != nil {
		channel := *state.Channel
		cloned.Channel = &channel
	}
	if state.PreviousChannel != nil {
		channel := *state.PreviousChannel
		cloned.PreviousChannel = &channel
	}
	if state.LastMessageNumbers != nil {
		cloned.LastMessageNumbers = make(map[string]uint64, len(state.LastMessageNumbers))
		for channelID, messageNumber := range state.LastMessageNumbers {
			cloned.LastMessageNumbers[channelID] = messageNumber
		}
	}
	return cloned
}

func channelTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func shutdownHTTPServer(ctx context.Context, server *http.Server) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down Drive changes server: %w", err)
	}
	return nil
}

func (s *driveChangesServer) ensureChannel(ctx context.Context) (time.Duration, error) {
	s.renewMu.Lock()
	defer s.renewMu.Unlock()

	pendingStopFailed := false
	if err := s.stopPreviousChannel(ctx); err != nil {
		pendingStopFailed = true
		s.warnf("drive changes serve: stop previous channel failed: %v", err)
	}

	s.mu.Lock()
	now := s.runtime.now().UTC()
	if s.currentChannelMatchesLocked() {
		delay := s.channelRenewalDelayLocked(now)
		if delay > 0 {
			if pendingStopFailed && delay > driveChangesRenewRetry {
				delay = driveChangesRenewRetry
			}
			s.mu.Unlock()
			return delay, nil
		}
	}
	if s.state.PreviousChannel != nil {
		s.mu.Unlock()
		return 0, errDriveChangesPreviousCleanupPending
	}
	pageToken := s.state.PageToken
	driveID := s.state.DriveID
	s.mu.Unlock()

	channelID, err := randomChannelID()
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	s.pendingChannel = channelID
	s.mu.Unlock()
	requestedExpiration := now.Add(s.channelTTL).UnixMilli()
	channel := &drive.Channel{
		Id:         channelID,
		Type:       "web_hook",
		Address:    s.webhookURL,
		Token:      s.channelToken,
		Expiration: requestedExpiration,
	}
	call := s.service.Changes.Watch(pageToken, channel).
		SupportsAllDrives(true).
		Context(ctx)
	if driveID != "" {
		call = call.DriveId(driveID)
	}
	response, err := call.Do()
	if err != nil {
		s.clearPendingChannel(channelID)
		return 0, err
	}
	if response == nil || strings.TrimSpace(response.Id) == "" || strings.TrimSpace(response.ResourceId) == "" {
		s.clearPendingChannel(channelID)
		return 0, errors.New("drive changes watch response missing channel identifiers")
	}
	expiration := response.Expiration
	if expiration <= 0 {
		expiration = requestedExpiration
	}

	s.mu.Lock()
	if s.pendingChannel == channelID {
		s.pendingChannel = ""
	}
	nextState := cloneDriveChangesServeState(s.state)
	nextState.PreviousChannel = nextState.Channel
	nextState.Channel = &driveChangesServeChannelState{
		ID:          strings.TrimSpace(response.Id),
		ResourceID:  strings.TrimSpace(response.ResourceId),
		ResourceURI: strings.TrimSpace(response.ResourceUri),
		Expiration:  expiration,
		WebhookURL:  s.webhookURL,
		TokenHash:   channelTokenHash(s.channelToken),
	}
	nextState.UpdatedAt = now.Format(time.RFC3339Nano)
	trimDriveChangesMessageNumbers(
		&nextState,
		driveChangesMessageKeyForChannel(nextState.Channel),
		driveChangesMessageKeyForChannel(nextState.PreviousChannel),
	)
	if err := writePollState(s.statePath, nextState); err != nil {
		channelToStop := *nextState.Channel
		s.mu.Unlock()
		_ = s.stopDriveChangesChannel(ctx, &channelToStop)
		return 0, err
	}
	s.state = nextState
	s.mu.Unlock()

	if err := s.stopPreviousChannel(ctx); err != nil {
		s.warnf("drive changes serve: stop previous channel failed: %v", err)
	}

	s.mu.Lock()
	delay := s.channelRenewalDelayLocked(now)
	if delay <= 0 {
		s.warnf(
			"drive changes serve: channel expiration is inside the renewal window; retrying renewal in %s",
			driveChangesRenewRetry,
		)
		delay = driveChangesRenewRetry
	}
	if s.state.PreviousChannel != nil && delay > driveChangesRenewRetry {
		delay = driveChangesRenewRetry
	}
	s.mu.Unlock()
	return delay, nil
}

func (s *driveChangesServer) clearPendingChannel(channelID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pendingChannel == channelID {
		s.pendingChannel = ""
	}
}

func (s *driveChangesServer) currentChannelMatchesLocked() bool {
	channel := s.state.Channel
	return channel != nil &&
		channel.ID != "" &&
		channel.ResourceID != "" &&
		channel.WebhookURL == s.webhookURL &&
		channel.TokenHash == channelTokenHash(s.channelToken)
}

func (s *driveChangesServer) channelRenewalDelayLocked(now time.Time) time.Duration {
	if s.state.Channel == nil || s.state.Channel.Expiration <= 0 {
		return 0
	}
	delay := time.UnixMilli(s.state.Channel.Expiration).Sub(now) - s.renewBefore
	if delay < time.Second {
		return 0
	}
	return delay
}

func (s *driveChangesServer) stopPreviousChannel(ctx context.Context) error {
	s.mu.Lock()
	previous := cloneDriveChangesServeChannel(s.state.PreviousChannel)
	s.mu.Unlock()
	if previous == nil {
		return nil
	}
	if err := s.stopDriveChangesChannel(ctx, previous); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.state.PreviousChannel
	if current == nil || current.ID != previous.ID || current.ResourceID != previous.ResourceID {
		return nil
	}
	nextState := cloneDriveChangesServeState(s.state)
	nextState.PreviousChannel = nil
	nextState.UpdatedAt = s.runtime.now().UTC().Format(time.RFC3339Nano)
	if err := writePollState(s.statePath, nextState); err != nil {
		return err
	}
	s.state = nextState
	return nil
}

func (s *driveChangesServer) stopDriveChangesChannel(ctx context.Context, channel *driveChangesServeChannelState) error {
	if channel == nil || channel.ID == "" || channel.ResourceID == "" {
		return nil
	}
	err := s.service.Channels.Stop(&drive.Channel{
		Id:         channel.ID,
		ResourceId: channel.ResourceID,
	}).Context(ctx).Do()
	if err != nil && !isNotFoundAPIError(err) {
		return err
	}
	return nil
}

func cloneDriveChangesServeChannel(channel *driveChangesServeChannelState) *driveChangesServeChannelState {
	if channel == nil {
		return nil
	}
	cloned := *channel
	return &cloned
}

func (s *driveChangesServer) runRenewLoop(ctx context.Context, delay time.Duration) {
	for {
		if delay < time.Second {
			delay = time.Second
		}
		if err := s.runtime.wait(ctx, delay); err != nil {
			return
		}
		nextDelay, err := s.ensureChannel(ctx)
		if err != nil {
			s.warnf("drive changes serve: channel renewal failed: %v", err)
			delay = driveChangesRenewRetry
			continue
		}
		delay = nextDelay
	}
}

func driveChangesMessageKeyForChannel(channel *driveChangesServeChannelState) string {
	if channel == nil {
		return ""
	}
	return driveChangesMessageKey(channel.ID, channel.ResourceID)
}

func trimDriveChangesMessageNumbers(state *driveChangesServeState, keepKeys ...string) {
	if len(state.LastMessageNumbers) <= 32 {
		return
	}
	keep := make(map[string]struct{}, len(keepKeys))
	for _, key := range keepKeys {
		if key != "" {
			keep[key] = struct{}{}
		}
	}
	for key := range state.LastMessageNumbers {
		if len(state.LastMessageNumbers) <= 32 {
			break
		}
		if _, ok := keep[key]; !ok {
			delete(state.LastMessageNumbers, key)
		}
	}
}
