package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/alecthomas/kong"
	"google.golang.org/api/gmail/v1"

	"github.com/steipete/gogcli/internal/authclient"
	"github.com/steipete/gogcli/internal/ui"
)

type GmailWatchPullCmd struct {
	Subscription  string   `name:"subscription" help:"Pub/Sub pull subscription (projects/.../subscriptions/...)"`
	FetchDelay    string   `name:"fetch-delay" help:"Delay before fetching Gmail history (seconds or duration)" default:"3s"`
	Timezone      string   `name:"timezone" short:"z" help:"Output timezone (IANA name, e.g. America/New_York, UTC). Default: GOG_TIMEZONE, config, then local"`
	Local         bool     `name:"local" help:"Use local timezone (default behavior, useful to override --timezone)"`
	HookURL       string   `name:"hook-url" help:"Webhook URL to forward messages"`
	HookToken     string   `name:"hook-token" help:"Webhook bearer token"`
	IncludeBody   bool     `name:"include-body" help:"Include text/plain body in hook payload"`
	MaxBytes      int      `name:"max-bytes" help:"Max bytes of body to include" default:"20000"`
	HistoryTypes  []string `name:"history-types" help:"History types to include (repeatable, comma-separated: messageAdded,messageDeleted,labelAdded,labelRemoved). Default: messageAdded"`
	ExcludeLabels string   `name:"exclude-labels" help:"List of Gmail label IDs to exclude from hook payload (e.g. SPAM,TRASH,Label_123). Set to empty string to disable." default:"SPAM,TRASH"`
	SaveHook      bool     `name:"save-hook" help:"Persist hook settings to watch state"`
}

func (c *GmailWatchPullCmd) Run(ctx context.Context, kctx *kong.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	subscription := strings.TrimSpace(c.Subscription)
	if subscription == "" {
		return usage("--subscription is required")
	}
	if _, subscriptionErr := projectIDFromPubSubSubscription(subscription); subscriptionErr != nil {
		return subscriptionErr
	}

	loc, err := resolveOutputLocation(ctx, c.Timezone, c.Local, stderrWriter(ctx))
	if err != nil {
		return err
	}
	historyTypes, err := parseHistoryTypes(c.HistoryTypes)
	if err != nil {
		return err
	}
	fetchDelay, err := parseDurationSeconds(c.FetchDelay)
	if err != nil {
		return err
	}
	if fetchDelay < 0 {
		return usage("--fetch-delay must be >= 0")
	}
	if dryRunErr := dryRunExit(ctx, flags, "gmail.watch.pull", map[string]any{
		"account":             account,
		"subscription":        subscription,
		"fetch_delay_seconds": fetchDelay.Seconds(),
		"history_types":       historyTypes,
		"exclude_labels":      splitCommaList(c.ExcludeLabels),
		"include_body":        c.IncludeBody,
		"max_bytes":           c.MaxBytes,
		"hook_url_set":        strings.TrimSpace(c.HookURL) != "",
		"hook_token_set":      c.HookToken != "",
		"save_hook":           c.SaveHook,
	}); dryRunErr != nil {
		return dryRunErr
	}

	store, err := loadGmailWatchStore(ctx, account)
	if err != nil {
		return err
	}
	state := store.Get()
	hook, err := resolveWatchHookFromFlags(kctx, state, watchHookFlagValues{
		URL:         c.HookURL,
		Token:       c.HookToken,
		IncludeBody: c.IncludeBody,
		MaxBytes:    c.MaxBytes,
	}, false)
	if err != nil {
		if errors.Is(err, errNoHookConfigured) {
			return usage("--hook-url is required unless stored watch state has a hook")
		}
		return err
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

	cfg := gmailWatchServeConfig{
		Account:       account,
		HookURL:       hook.URL,
		HookToken:     hook.Token,
		HookTimeout:   defaultHookRequestTimeoutSec * time.Second,
		HistoryMax:    defaultHistoryMaxResults,
		ResyncMax:     defaultHistoryResyncMax,
		FetchDelay:    fetchDelay,
		HistoryTypes:  historyTypes,
		IncludeBody:   hook.IncludeBody,
		MaxBodyBytes:  hook.MaxBytes,
		DateLocation:  loc,
		ExcludeLabels: splitCommaList(c.ExcludeLabels),
		VerboseOutput: flags.Verbose,
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultHookMaxBytes
	}

	selectedClient := strings.TrimSpace(flags.Client)
	gmailFactory := gmailServiceFactory(ctx)
	serviceFactory := func(ctx context.Context, account string) (*gmail.Service, error) {
		if selectedClient != "" {
			ctx = authclient.WithClient(ctx, selectedClient)
		}
		return gmailFactory(ctx, account)
	}

	receiver, err := newGmailPubSubReceiver(ctx, subscription, gmailPubSubReceiveSettings{
		MaxOutstandingMessages: 1,
	})
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := receiver.Close(); closeErr != nil {
			u.Err().Linef("watch: failed to close Pub/Sub receiver: %v", closeErr)
		}
	}()

	processor := &gmailWatchServer{
		cfg:             cfg,
		store:           store,
		newService:      serviceFactory,
		hookClient:      &http.Client{Timeout: cfg.HookTimeout},
		excludeLabelIDs: stringSet(cfg.ExcludeLabels),
		logf:            u.Err().Linef,
		warnf:           u.Err().Linef,
	}
	u.Err().Linef("watch: pulling from %s", subscription)

	err = receiver.Receive(ctx, processor.handlePullMessage)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

type gmailPubSubReceiveSettings struct {
	MaxOutstandingMessages int
}

type gmailPubSubReceiver interface {
	Receive(context.Context, func(context.Context, *gmailPubSubMessage)) error
	Close() error
}

type gmailPubSubMessage struct {
	ID         string
	Data       []byte
	Attributes map[string]string
	ack        func()
	nack       func()
}

func (m *gmailPubSubMessage) Ack() {
	if m.ack != nil {
		m.ack()
	}
}

func (m *gmailPubSubMessage) Nack() {
	if m.nack != nil {
		m.nack()
	}
}

var newGmailPubSubReceiver = newGoogleGmailPubSubReceiver

type googleGmailPubSubReceiver struct {
	client     *pubsub.Client
	subscriber *pubsub.Subscriber
}

func newGoogleGmailPubSubReceiver(ctx context.Context, subscription string, settings gmailPubSubReceiveSettings) (gmailPubSubReceiver, error) {
	projectID, err := projectIDFromPubSubSubscription(subscription)
	if err != nil {
		return nil, err
	}
	client, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	subscriber := client.Subscriber(subscription)
	if settings.MaxOutstandingMessages > 0 {
		subscriber.ReceiveSettings.MaxOutstandingMessages = settings.MaxOutstandingMessages
	}
	subscriber.ReceiveSettings.NumGoroutines = 1
	return &googleGmailPubSubReceiver{client: client, subscriber: subscriber}, nil
}

func (r *googleGmailPubSubReceiver) Receive(ctx context.Context, f func(context.Context, *gmailPubSubMessage)) error {
	return r.subscriber.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		f(ctx, &gmailPubSubMessage{
			ID:         msg.ID,
			Data:       msg.Data,
			Attributes: msg.Attributes,
			ack:        msg.Ack,
			nack:       msg.Nack,
		})
	})
}

func (r *googleGmailPubSubReceiver) Close() error {
	return r.client.Close()
}

func projectIDFromPubSubSubscription(subscription string) (string, error) {
	parts := strings.Split(strings.TrimSpace(subscription), "/")
	if len(parts) == 4 &&
		parts[0] == "projects" &&
		parts[1] != "" &&
		parts[2] == "subscriptions" &&
		parts[3] != "" {
		return parts[1], nil
	}
	return "", usage("--subscription must be projects/{project}/subscriptions/{subscription}")
}

func decodeGmailPullPayload(msg *gmailPubSubMessage) (gmailPushPayload, error) {
	if msg == nil || len(msg.Data) == 0 {
		return gmailPushPayload{}, errors.New("missing message data")
	}
	var payload gmailPushPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return gmailPushPayload{}, err
	}
	payload.MessageID = strings.TrimSpace(msg.ID)
	return payload, nil
}

func (s *gmailWatchServer) handlePullMessage(ctx context.Context, msg *gmailPubSubMessage) {
	payload, err := decodeGmailPullPayload(msg)
	if err != nil {
		s.warnf("watch: invalid pull data: %v", err)
		msg.Ack()
		return
	}
	if payload.EmailAddress != "" && !strings.EqualFold(payload.EmailAddress, s.cfg.Account) {
		s.warnf("watch: ignoring pull notification for %s", payload.EmailAddress)
		msg.Ack()
		return
	}

	_, err = s.processGmailWatchPayload(ctx, payload)
	if err == nil || errors.Is(err, errNoNewMessages) {
		msg.Ack()
		return
	}
	var rateErr *gmailWatchRateLimitError
	if errors.As(err, &rateErr) {
		s.warnf("watch: Gmail rate limit circuit open: %v", err)
		msg.Nack()
		return
	}
	s.warnf("watch: handle pull failed: %v", err)
	msg.Nack()
}

func (s *gmailWatchServer) processGmailWatchPayload(ctx context.Context, payload gmailPushPayload) (*gmailWatchProcessedPayload, error) {
	var progressBefore gmailWatchState
	if s.store != nil {
		progressBefore = s.store.Get()
	}
	result, err := s.handlePush(ctx, payload)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errNoNewMessages
	}
	processed := &gmailWatchProcessedPayload{Payload: result}
	if s.cfg.HookURL == "" {
		return processed, nil
	}
	if err := s.sendHook(ctx, result); err != nil {
		s.warnf("watch: hook failed: %v", err)
		processed.HookFailed = true
		if restoreErr := s.restoreWatchProgressForRetry(progressBefore, result.HistoryID, payload.MessageID); restoreErr != nil {
			s.warnf("watch: failed to preserve retry state after hook failure: %v", restoreErr)
		}
		return processed, &gmailWatchHookDeliveryError{Err: err}
	}
	return processed, nil
}

type gmailWatchProcessedPayload struct {
	Payload    *gmailHookPayload
	HookFailed bool
}

type gmailWatchHookDeliveryError struct {
	Err error
}

func (e *gmailWatchHookDeliveryError) Error() string {
	return fmt.Sprintf("hook delivery failed: %v", e.Err)
}

func (e *gmailWatchHookDeliveryError) Unwrap() error {
	return e.Err
}

func (s *gmailWatchServer) restoreWatchProgressForRetry(before gmailWatchState, historyID, pushMessageID string) error {
	if s.store == nil {
		return nil
	}
	return s.store.Update(func(state *gmailWatchState) error {
		if state.HistoryID != historyID {
			return nil
		}
		if pushMessageID != "" && state.LastPushMessageID != pushMessageID {
			return nil
		}
		state.HistoryID = before.HistoryID
		state.LastPushMessageID = before.LastPushMessageID
		return nil
	})
}
