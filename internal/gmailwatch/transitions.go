package gmailwatch

import "time"

const (
	DeliveryStatusError     = "error"
	DeliveryStatusHTTPError = "http_error"
	DeliveryStatusOK        = "ok"
	DeliveryStatusRateLimit = "rate_limited"
)

func (r *Repository) SetHook(hook *Hook, now time.Time) error {
	return r.Update(func(state *State) error {
		if hook == nil {
			state.Hook = nil
		} else {
			cloned := *hook
			state.Hook = &cloned
		}
		state.UpdatedAtMs = now.UnixMilli()

		return nil
	})
}

func (r *Repository) AdvanceHistory(historyID, pushMessageID string, now time.Time) error {
	return r.Update(func(state *State) error {
		return AdvanceHistory(state, historyID, pushMessageID, now)
	})
}

func (r *Repository) RestoreProgress(before State, historyID, pushMessageID string) (bool, error) {
	restored := false

	err := r.Update(func(state *State) error {
		restored = RestoreProgress(state, before, historyID, pushMessageID)

		return nil
	})
	if err != nil {
		return false, err
	}

	return restored, nil
}

func (r *Repository) CheckRateLimit(now time.Time) (time.Time, bool, error) {
	state := r.Get()
	if state.RateLimitedUntilMs <= 0 {
		return time.Time{}, false, nil
	}

	until := time.UnixMilli(state.RateLimitedUntilMs)
	if until.After(now) {
		return until, true, nil
	}

	open := false

	err := r.Update(func(state *State) error {
		if state.RateLimitedUntilMs <= 0 {
			return nil
		}

		currentUntil := time.UnixMilli(state.RateLimitedUntilMs)
		if currentUntil.After(now) {
			until = currentUntil
			open = true

			return nil
		}

		state.RateLimitedUntilMs = 0
		if state.LastDeliveryStatus == DeliveryStatusRateLimit {
			state.LastDeliveryStatusNote = ""
		}

		return nil
	})
	if err != nil {
		return time.Time{}, false, err
	}

	return until, open, nil
}

func (r *Repository) OpenRateLimit(until time.Time, note string, now time.Time) error {
	return r.Update(func(state *State) error {
		untilMs := until.UnixMilli()
		if untilMs > state.RateLimitedUntilMs {
			state.RateLimitedUntilMs = untilMs
		}
		state.LastDeliveryStatus = DeliveryStatusRateLimit
		state.LastDeliveryAtMs = now.UnixMilli()
		state.LastDeliveryStatusNote = note

		return nil
	})
}

func (r *Repository) RecordDelivery(status, note string, now time.Time) error {
	return r.Update(func(state *State) error {
		state.LastDeliveryStatus = status
		state.LastDeliveryAtMs = now.UnixMilli()
		state.LastDeliveryStatusNote = note

		return nil
	})
}
