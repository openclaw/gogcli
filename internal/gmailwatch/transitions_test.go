package gmailwatch

import (
	"testing"
	"time"
)

func TestRepositoryNamedTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	repository := NewMemory(State{
		HistoryID:          "100",
		LastPushMessageID:  "before",
		RateLimitedUntilMs: 123,
	}, Options{})

	hook := &Hook{URL: "https://example.com/hook", Token: "secret"}
	if err := repository.SetHook(hook, now); err != nil {
		t.Fatalf("SetHook: %v", err)
	}
	hook.Token = "mutated"

	state := repository.Get()
	if state.Hook == nil || state.Hook.Token != "secret" || state.UpdatedAtMs != now.UnixMilli() {
		t.Fatalf("hook state = %#v", state)
	}

	if err := repository.AdvanceHistory("200", "push", now.Add(time.Second)); err != nil {
		t.Fatalf("AdvanceHistory: %v", err)
	}

	advanced := repository.Get()
	if advanced.HistoryID != "200" || advanced.LastPushMessageID != "push" {
		t.Fatalf("advanced state = %#v", advanced)
	}

	if advanced.RateLimitedUntilMs != 123 {
		t.Fatalf("advance changed rate limit: %#v", advanced)
	}

	restored, err := repository.RestoreProgress(state, "200", "push")
	if err != nil {
		t.Fatalf("RestoreProgress: %v", err)
	}

	if !restored {
		t.Fatal("RestoreProgress did not restore matching progress")
	}

	restoredState := repository.Get()
	if restoredState.HistoryID != "100" || restoredState.LastPushMessageID != "before" {
		t.Fatalf("restored state = %#v", restoredState)
	}

	if _, err := repository.RestoreProgress(state, "200", "other"); err != nil {
		t.Fatalf("RestoreProgress mismatch: %v", err)
	}
}

func TestRepositoryRateLimitTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	repository := NewMemory(State{}, Options{})

	firstUntil := now.Add(time.Minute)
	if err := repository.OpenRateLimit(firstUntil, "first", now); err != nil {
		t.Fatalf("OpenRateLimit: %v", err)
	}

	until, open, err := repository.CheckRateLimit(now.Add(30 * time.Second))
	if err != nil {
		t.Fatalf("CheckRateLimit open: %v", err)
	}

	if !open || !until.Equal(firstUntil) {
		t.Fatalf("open = %t, until = %s", open, until)
	}

	earlierUntil := now.Add(45 * time.Second)
	if openErr := repository.OpenRateLimit(earlierUntil, "second", now.Add(time.Second)); openErr != nil {
		t.Fatalf("OpenRateLimit earlier: %v", openErr)
	}

	if got := repository.Get().RateLimitedUntilMs; got != firstUntil.UnixMilli() {
		t.Fatalf("rate limit moved backward: %d", got)
	}

	until, open, err = repository.CheckRateLimit(now.Add(2 * time.Minute))
	if err != nil {
		t.Fatalf("CheckRateLimit expired: %v", err)
	}

	if open || !until.Equal(firstUntil) {
		t.Fatalf("expired open = %t, until = %s", open, until)
	}

	state := repository.Get()
	if state.RateLimitedUntilMs != 0 || state.LastDeliveryStatusNote != "" {
		t.Fatalf("expired state = %#v", state)
	}
}

func TestRepositoryRecordDelivery(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	repository := NewMemory(State{}, Options{})

	if err := repository.RecordDelivery(DeliveryStatusHTTPError, "status 502", now); err != nil {
		t.Fatalf("RecordDelivery: %v", err)
	}

	state := repository.Get()
	if state.LastDeliveryStatus != DeliveryStatusHTTPError ||
		state.LastDeliveryStatusNote != "status 502" ||
		state.LastDeliveryAtMs != now.UnixMilli() {
		t.Fatalf("delivery state = %#v", state)
	}
}
