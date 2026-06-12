package cmd

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
)

var (
	errDriveChangesDuplicateNotification = errors.New("duplicate drive changes notification")
	errDriveChangesIgnoredNotification   = errors.New("ignored drive changes notification")
	errDriveChangesUntrackedNotification = errors.New("untracked drive changes notification channel")
)

type driveChangesNotification struct {
	ChannelID         string
	ResourceID        string
	ResourceState     string
	ResourceURI       string
	Changed           string
	ChannelExpiration string
	MessageNumber     uint64
}

type driveChangesServeEvent struct {
	Kind              string          `json:"kind"`
	ChannelID         string          `json:"channelId"`
	ResourceID        string          `json:"resourceId"`
	ResourceState     string          `json:"resourceState"`
	ResourceURI       string          `json:"resourceUri"`
	Changed           string          `json:"changed,omitempty"`
	ChannelExpiration string          `json:"channelExpiration,omitempty"`
	MessageNumber     uint64          `json:"messageNumber"`
	DriveID           string          `json:"driveId,omitempty"`
	PageToken         string          `json:"pageToken"`
	NextPageToken     string          `json:"nextPageToken"`
	Changes           []*drive.Change `json:"changes"`
}

func (s *driveChangesServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != s.path {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !driveChangesChannelTokenMatches(r.Header.Get("X-Goog-Channel-Token"), s.channelToken) {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	notification, err := parseDriveChangesNotification(r)
	if err != nil {
		s.warnf("drive changes serve: invalid notification: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	notificationCtx, cancel := s.notificationContext(r.Context())
	defer cancel()
	if err := s.handleNotification(notificationCtx, notification); err != nil {
		if errors.Is(err, errDriveChangesUntrackedNotification) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if errors.Is(err, errDriveChangesDuplicateNotification) || errors.Is(err, errDriveChangesIgnoredNotification) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		s.warnf("drive changes serve: notification failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *driveChangesServer) notificationContext(requestCtx context.Context) (context.Context, context.CancelFunc) {
	timeout := s.notificationTimeout
	if timeout <= 0 {
		timeout = defaultDriveChangesNotificationTimeout
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), timeout)
	if s.runDone == nil {
		return ctx, cancel
	}
	finished := make(chan struct{}, 1)
	go func() {
		select {
		case <-s.runDone:
			cancel()
		case <-finished:
		}
	}()
	return ctx, func() {
		select {
		case finished <- struct{}{}:
		default:
		}
		cancel()
	}
}

func (s *driveChangesServer) acquireNotification(ctx context.Context) error {
	s.notificationOnce.Do(func() {
		s.notificationGate = make(chan struct{}, 1)
	})
	select {
	case s.notificationGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *driveChangesServer) releaseNotification() {
	<-s.notificationGate
}

func driveChangesChannelTokenMatches(got string, expected string) bool {
	got = strings.TrimSpace(got)
	expected = strings.TrimSpace(expected)
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func parseDriveChangesNotification(r *http.Request) (driveChangesNotification, error) {
	channelID := strings.TrimSpace(r.Header.Get("X-Goog-Channel-ID"))
	resourceID := strings.TrimSpace(r.Header.Get("X-Goog-Resource-ID"))
	resourceState := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Goog-Resource-State")))
	resourceURI := strings.TrimSpace(r.Header.Get("X-Goog-Resource-URI"))
	messageNumberRaw := strings.TrimSpace(r.Header.Get("X-Goog-Message-Number"))
	changed := strings.TrimSpace(r.Header.Get("X-Goog-Changed"))
	channelExpiration := strings.TrimSpace(r.Header.Get("X-Goog-Channel-Expiration"))
	switch {
	case channelID == "":
		return driveChangesNotification{}, errors.New("missing X-Goog-Channel-ID")
	case len(channelID) > 128:
		return driveChangesNotification{}, errors.New("x-goog-channel-id is too long")
	case resourceID == "":
		return driveChangesNotification{}, errors.New("missing X-Goog-Resource-ID")
	case len(resourceID) > 512:
		return driveChangesNotification{}, errors.New("x-goog-resource-id is too long")
	case resourceState == "":
		return driveChangesNotification{}, errors.New("missing X-Goog-Resource-State")
	case len(resourceState) > 64:
		return driveChangesNotification{}, errors.New("x-goog-resource-state is too long")
	case resourceURI == "":
		return driveChangesNotification{}, errors.New("missing X-Goog-Resource-URI")
	case len(resourceURI) > 4096:
		return driveChangesNotification{}, errors.New("x-goog-resource-uri is too long")
	case messageNumberRaw == "":
		return driveChangesNotification{}, errors.New("missing X-Goog-Message-Number")
	case len(changed) > 1024:
		return driveChangesNotification{}, errors.New("x-goog-changed is too long")
	case len(channelExpiration) > 256:
		return driveChangesNotification{}, errors.New("x-goog-channel-expiration is too long")
	}
	messageNumber, err := strconv.ParseUint(messageNumberRaw, 10, 64)
	if err != nil || messageNumber == 0 {
		return driveChangesNotification{}, errors.New("invalid X-Goog-Message-Number")
	}
	return driveChangesNotification{
		ChannelID:         channelID,
		ResourceID:        resourceID,
		ResourceState:     resourceState,
		ResourceURI:       resourceURI,
		Changed:           changed,
		ChannelExpiration: channelExpiration,
		MessageNumber:     messageNumber,
	}, nil
}

func (s *driveChangesServer) handleNotification(ctx context.Context, notification driveChangesNotification) error {
	if err := s.acquireNotification(ctx); err != nil {
		return err
	}
	defer s.releaseNotification()

	s.mu.Lock()
	if !s.notificationChannelAllowedLocked(notification) {
		s.warnf(
			"drive changes serve: rejecting untracked channel=%s resource=%s",
			notification.ChannelID,
			notification.ResourceID,
		)
		s.mu.Unlock()
		return errDriveChangesUntrackedNotification
	}
	messageKey := driveChangesMessageKey(notification.ChannelID, notification.ResourceID)
	if last := s.state.LastMessageNumbers[messageKey]; last >= notification.MessageNumber {
		s.logf(
			"drive changes serve: ignoring duplicate channel=%s message=%d",
			notification.ChannelID,
			notification.MessageNumber,
		)
		s.mu.Unlock()
		return errDriveChangesDuplicateNotification
	}
	if notification.ResourceState == "sync" {
		err := s.acknowledgeNotificationLocked(notification)
		s.mu.Unlock()
		if err != nil {
			return err
		}
		return errDriveChangesIgnoredNotification
	}

	pageToken := s.state.PageToken
	driveID := s.state.DriveID
	s.mu.Unlock()

	changes, nextPageToken, err := loadDriveChanges(ctx, s.service, pageToken, driveChangesLoadOptions{
		max:            s.max,
		includeRemoved: s.includeRemoved,
		driveID:        driveID,
		all:            true,
	})
	if err != nil {
		return err
	}
	filtered := filterDriveChangesByFile(changes, s.filterFile)
	if len(filtered) > 0 && s.onChange != "" {
		event := driveChangesServeEvent{
			Kind:              "drive_changes_notification",
			ChannelID:         notification.ChannelID,
			ResourceID:        notification.ResourceID,
			ResourceState:     notification.ResourceState,
			ResourceURI:       notification.ResourceURI,
			Changed:           notification.Changed,
			ChannelExpiration: notification.ChannelExpiration,
			MessageNumber:     notification.MessageNumber,
			DriveID:           driveID,
			PageToken:         pageToken,
			NextPageToken:     nextPageToken,
			Changes:           filtered,
		}
		if err := s.runtime.runHook(ctx, s.onChange, event); err != nil {
			return err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.PageToken != pageToken {
		return errors.New("drive changes serve state advanced during notification processing")
	}
	nextState := cloneDriveChangesServeState(s.state)
	nextState.PageToken = nextPageToken
	nextState.UpdatedAt = s.runtime.now().UTC().Format(time.RFC3339Nano)
	setDriveChangesMessageNumber(&nextState, notification, notification.MessageNumber)
	if err := writePollState(s.statePath, nextState); err != nil {
		return err
	}
	s.state = nextState
	return nil
}

func (s *driveChangesServer) notificationChannelAllowedLocked(notification driveChangesNotification) bool {
	if !s.autoRenew {
		return true
	}
	tracked := false
	for _, channel := range []*driveChangesServeChannelState{s.state.Channel, s.state.PreviousChannel} {
		if channel == nil {
			continue
		}
		tracked = true
		if channel.ID == notification.ChannelID && channel.ResourceID == notification.ResourceID {
			return true
		}
	}
	if s.pendingChannel != "" {
		tracked = true
		if s.pendingChannel == notification.ChannelID {
			return true
		}
	}
	return !tracked
}

func (s *driveChangesServer) acknowledgeNotificationLocked(notification driveChangesNotification) error {
	nextState := cloneDriveChangesServeState(s.state)
	nextState.UpdatedAt = s.runtime.now().UTC().Format(time.RFC3339Nano)
	setDriveChangesMessageNumber(&nextState, notification, notification.MessageNumber)
	if err := writePollState(s.statePath, nextState); err != nil {
		return err
	}
	s.state = nextState
	return nil
}

func setDriveChangesMessageNumber(state *driveChangesServeState, notification driveChangesNotification, messageNumber uint64) {
	if state.LastMessageNumbers == nil {
		state.LastMessageNumbers = make(map[string]uint64)
	}
	messageKey := driveChangesMessageKey(notification.ChannelID, notification.ResourceID)
	state.LastMessageNumbers[messageKey] = messageNumber
	trimDriveChangesMessageNumbers(
		state,
		driveChangesMessageKeyForChannel(state.Channel),
		driveChangesMessageKeyForChannel(state.PreviousChannel),
		messageKey,
	)
}

func driveChangesMessageKey(channelID string, resourceID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(channelID)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(resourceID))
}
