package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/idtoken"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

var errNoNewMessages = gmailwatch.ErrNoNewMessages

const (
	gmailWatchStatusHTTPError = gmailwatch.DeliveryStatusHTTPError
	gmailWatchStatusRateLimit = gmailwatch.DeliveryStatusRateLimit
)

type gmailWatchRateLimitError = gmailwatch.RateLimitError

type gmailWatchServer struct {
	cfg             gmailWatchServeConfig
	store           *gmailWatchStore
	validator       *idtoken.Validator
	newService      func(context.Context, string) (*gmail.Service, error)
	sleep           func(context.Context, time.Duration) error
	hookClient      *http.Client
	excludeLabelIDs map[string]struct{}
	logf            func(string, ...any)
	warnf           func(string, ...any)
	now             func() time.Time
}

func (s *gmailWatchServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := gmailwatch.HTTPHandler{
		Config: gmailwatch.HTTPConfig{
			Path:        s.cfg.Path,
			Account:     s.cfg.Account,
			BodyLimit:   defaultPushBodyLimitBytes,
			HasHook:     s.cfg.HookURL != "",
			AllowNoHook: s.cfg.AllowNoHook,
		},
		Authorize: s.authorize,
		Process: func(ctx context.Context, notification gmailwatch.Notification) (*gmailwatch.ProcessedPayload, error) {
			return s.watchProcessor().Process(ctx, notification)
		},
		Now:   s.currentTime,
		Warnf: s.warnf,
	}
	handler.ServeHTTP(w, r)
}

func (s *gmailWatchServer) authorize(r *http.Request) bool {
	authorizer := gmailwatch.Authorizer{
		Config: gmailwatch.AuthConfig{
			VerifyOIDC:   s.cfg.VerifyOIDC,
			OIDCEmail:    s.cfg.OIDCEmail,
			OIDCAudience: s.cfg.OIDCAudience,
			SharedToken:  s.cfg.SharedToken,
		},
		Verify: func(ctx context.Context, token, audience, email string) (bool, error) {
			return verifyOIDCToken(ctx, s.validator, token, audience, email)
		},
		Warnf: s.warnf,
	}

	return authorizer.Authorize(r)
}

func (s *gmailWatchServer) sendHook(ctx context.Context, payload *gmailHookPayload) error {
	delivery := s.deliverHook(ctx, payload)
	if delivery.Record {
		_ = s.store.RecordDelivery(delivery.Status, delivery.Note, s.currentTime())
	}

	return delivery.Err
}

func (s *gmailWatchServer) deliverHook(ctx context.Context, payload *gmailHookPayload) gmailwatch.DeliveryResult {
	sender := gmailwatch.HookSender{
		URL:    s.cfg.HookURL,
		Token:  s.cfg.HookToken,
		Client: s.hookClient,
	}

	return sender.Send(ctx, payload)
}

func verifyOIDCToken(ctx context.Context, validator *idtoken.Validator, token, audience, expectedEmail string) (bool, error) {
	if validator == nil {
		return false, errors.New("no OIDC validator")
	}
	payload, err := validator.Validate(ctx, token, audience)
	if err != nil {
		return false, err
	}
	if expectedEmail == "" {
		return true, nil
	}
	email, _ := payload.Claims["email"].(string)
	if !strings.EqualFold(email, expectedEmail) {
		return false, fmt.Errorf("oidc email mismatch: %s", email)
	}
	return true, nil
}

func (s *gmailWatchServer) currentTime() time.Time {
	if s.now != nil {
		return s.now()
	}

	return time.Now()
}

func isStaleHistoryError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		if gerr.Code == http.StatusBadRequest || gerr.Code == http.StatusNotFound {
			msg := strings.ToLower(gerr.Message)
			if strings.Contains(msg, "history") {
				return true
			}
			for _, item := range gerr.Errors {
				if strings.Contains(strings.ToLower(item.Message), "history") {
					return true
				}
				if gerr.Code == http.StatusNotFound && strings.EqualFold(strings.TrimSpace(item.Reason), "notfound") {
					return true
				}
			}
			if gerr.Code == http.StatusNotFound && strings.Contains(msg, "not found") {
				return true
			}
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), "history")
}

func isNotFoundAPIError(err error) bool {
	var gerr *googleapi.Error
	if errors.As(err, &gerr) {
		return gerr.Code == http.StatusNotFound
	}
	return false
}

func gmailWatchRateLimitUntil(err error, now time.Time) (time.Time, bool) {
	var gerr *googleapi.Error
	if !errors.As(err, &gerr) || gerr.Code != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	if until, ok := parseRetryAfterUntil(gerr.Header.Get("Retry-After"), now); ok {
		return until, true
	}
	return now.Add(time.Minute), true
}

func parseRetryAfterUntil(raw string, now time.Time) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		return now.Add(time.Duration(seconds) * time.Second), true
	}
	if parsed, err := http.ParseTime(trimmed); err == nil {
		return parsed, true
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}
