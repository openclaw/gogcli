//nolint:tagliatelle // Persisted Gmail watch schemas retain their existing camelCase keys.
package gmailwatch

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrHistoryIDRequired = errors.New("historyId is required")
	ErrInvalidHistoryID  = errors.New("invalid historyId")
)

type Hook struct {
	URL         string `json:"url"`
	Token       string `json:"token,omitempty"`
	IncludeBody bool   `json:"includeBody,omitempty"`
	MaxBytes    int    `json:"maxBytes,omitempty"`
}

type State struct {
	Account                string   `json:"account"`
	Topic                  string   `json:"topic"`
	Labels                 []string `json:"labels,omitempty"`
	HistoryID              string   `json:"historyId"`
	ExpirationMs           int64    `json:"expirationMs,omitempty"`
	ProviderExpirationMs   int64    `json:"providerExpirationMs,omitempty"`
	RenewAfterMs           int64    `json:"renewAfterMs,omitempty"`
	UpdatedAtMs            int64    `json:"updatedAtMs,omitempty"`
	Hook                   *Hook    `json:"hook,omitempty"`
	LastDeliveryStatus     string   `json:"lastDeliveryStatus,omitempty"`
	LastDeliveryAtMs       int64    `json:"lastDeliveryAtMs,omitempty"`
	LastDeliveryStatusNote string   `json:"lastDeliveryStatusNote,omitempty"`
	LastPushMessageID      string   `json:"lastPushMessageId,omitempty"`
	RateLimitedUntilMs     int64    `json:"rateLimitedUntilMs,omitempty"`
}

func ParseHistoryID(raw string) (uint64, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, ErrHistoryIDRequired
	}

	id, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w %q", ErrInvalidHistoryID, trimmed)
	}

	return id, nil
}

func FormatHistoryID(id uint64) string {
	if id == 0 {
		return ""
	}

	return strconv.FormatUint(id, 10)
}

func IsStaleHistoryID(currentRaw, candidateRaw string) (bool, error) {
	currentID, candidateID, currentOK, candidateOK, err := compareHistoryIDs(currentRaw, candidateRaw)
	if err != nil {
		return false, err
	}

	if !currentOK || !candidateOK {
		return false, nil
	}

	return candidateID <= currentID, nil
}

func AdvanceHistory(state *State, historyID, pushMessageID string, now time.Time) error {
	shouldUpdate, err := shouldUpdateHistoryID(state.HistoryID, historyID)
	if err != nil {
		return err
	}

	if shouldUpdate {
		state.HistoryID = historyID
	}

	if pushMessageID != "" {
		state.LastPushMessageID = pushMessageID
	}
	state.UpdatedAtMs = now.UnixMilli()

	return nil
}

func RestoreProgress(state *State, before State, historyID, pushMessageID string) bool {
	if state.HistoryID != historyID {
		return false
	}

	if pushMessageID != "" && state.LastPushMessageID != pushMessageID {
		return false
	}

	state.HistoryID = before.HistoryID
	state.LastPushMessageID = before.LastPushMessageID

	return true
}

func parseHistoryIDOptional(raw string) (uint64, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, false, nil
	}

	id, err := ParseHistoryID(trimmed)
	if err != nil {
		return 0, true, err
	}

	return id, true, nil
}

func compareHistoryIDs(storedRaw, candidateRaw string) (storedID, candidateID uint64, storedOK, candidateOK bool, err error) {
	storedID, storedOK, err = parseHistoryIDOptional(storedRaw)
	if err != nil {
		return 0, 0, false, false, err
	}

	candidateID, candidateOK, err = parseHistoryIDOptional(candidateRaw)
	if err != nil {
		return storedID, 0, storedOK, true, err
	}

	return storedID, candidateID, storedOK, candidateOK, nil
}

func shouldUpdateHistoryID(currentRaw, candidateRaw string) (bool, error) {
	currentID, candidateID, currentOK, candidateOK, err := compareHistoryIDs(currentRaw, candidateRaw)
	if err != nil {
		return false, err
	}

	if !candidateOK {
		return false, nil
	}

	if !currentOK {
		return true, nil
	}

	return candidateID >= currentID, nil
}

func cloneState(state State) State {
	cloned := state

	cloned.Labels = append([]string(nil), state.Labels...)
	if state.Hook != nil {
		hook := *state.Hook
		cloned.Hook = &hook
	}

	return cloned
}
