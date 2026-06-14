package gmailwatch

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrNoNewMessages        = errors.New("no new messages")
	ErrMissingRepository    = errors.New("missing watch repository")
	ErrMissingSourceFactory = errors.New("missing watch source factory")
	ErrMissingSource        = errors.New("missing watch source")
)

type Notification struct {
	HistoryID string
	MessageID string
}

type ProcessorConfig struct {
	Account      string
	HistoryMax   int64
	ResyncMax    int64
	FetchDelay   time.Duration
	HistoryTypes []string
	Verbose      bool
}

type RateLimitError struct {
	Until time.Time
	Cause error
}

func (e *RateLimitError) Error() string {
	if e.Until.IsZero() {
		return "gmail watch rate limited"
	}

	return fmt.Sprintf("gmail watch rate limited until %s", e.Until.Format(time.RFC3339))
}

func (e *RateLimitError) Unwrap() error {
	return e.Cause
}

type DeliveryResult struct {
	Status string
	Note   string
	Err    error
	Record bool
}

type ProcessedPayload struct {
	Payload    *Payload
	HookFailed bool
}

type HookDeliveryError struct {
	Err error
}

func (e *HookDeliveryError) Error() string {
	return fmt.Sprintf("hook delivery failed: %v", e.Err)
}

func (e *HookDeliveryError) Unwrap() error {
	return e.Err
}

type Processor struct {
	Config              ProcessorConfig
	Repository          *Repository
	NewSource           func(context.Context) (Source, error)
	Deliver             func(context.Context, *Payload) DeliveryResult
	Now                 func() time.Time
	Sleep               func(context.Context, time.Duration) error
	IsStaleHistoryError func(error) bool
	RateLimitUntil      func(error, time.Time) (time.Time, bool)
	Logf                func(string, ...any)
	Warnf               func(string, ...any)
}

func (p *Processor) Process(ctx context.Context, notification Notification) (*ProcessedPayload, error) {
	if p.Repository == nil {
		return nil, ErrMissingRepository
	}

	progressBefore := p.Repository.Get()

	payload, err := p.Handle(ctx, notification)
	if err != nil {
		return nil, err
	}

	if payload == nil {
		return nil, ErrNoNewMessages
	}

	processed := &ProcessedPayload{Payload: payload}
	if p.Deliver == nil {
		return processed, nil
	}

	delivery := p.Deliver(ctx, payload)
	if delivery.Record {
		if recordErr := p.Repository.RecordDelivery(delivery.Status, delivery.Note, p.currentTime()); recordErr != nil {
			p.warnf("watch: failed to update delivery state: %v", recordErr)
		}
	}

	if delivery.Err == nil {
		return processed, nil
	}

	p.warnf("watch: hook failed: %v", delivery.Err)
	processed.HookFailed = true

	if _, restoreErr := p.Repository.RestoreProgress(progressBefore, payload.HistoryID, notification.MessageID); restoreErr != nil {
		p.warnf("watch: failed to preserve retry state after hook failure: %v", restoreErr)
	}

	return processed, &HookDeliveryError{Err: delivery.Err}
}

func (p *Processor) Handle(ctx context.Context, notification Notification) (*Payload, error) {
	if p.Repository == nil {
		return nil, ErrMissingRepository
	}

	if notification.MessageID != "" {
		state := p.Repository.Get()
		if state.LastPushMessageID == notification.MessageID {
			p.logf("watch: ignoring duplicate push %s", notification.MessageID)

			return nil, ErrNoNewMessages
		}
	}

	if err := p.checkRateLimitCircuit(p.currentTime()); err != nil {
		return nil, err
	}

	startID, err := p.Repository.StartHistoryID(notification.HistoryID)
	if err != nil {
		return nil, err
	}

	if startID == 0 {
		p.logStaleNotification(notification.HistoryID)

		return nil, ErrNoNewMessages
	}

	if sleepErr := p.sleepForFetch(ctx); sleepErr != nil {
		return nil, sleepErr
	}

	if p.NewSource == nil {
		return nil, ErrMissingSourceFactory
	}

	source, err := p.NewSource(ctx)
	if err != nil {
		return nil, err
	}

	if source == nil {
		return nil, ErrMissingSource
	}

	historyPage, err := source.ListHistory(ctx, startID, p.Config.HistoryMax, p.Config.HistoryTypes)
	if err != nil {
		if p.IsStaleHistoryError != nil && p.IsStaleHistoryError(err) {
			return p.resync(ctx, source, notification)
		}

		return nil, p.openRateLimitCircuitIfNeeded(err)
	}

	nextHistoryID := notification.HistoryID
	if historyPage.HistoryID != "" {
		nextHistoryID = historyPage.HistoryID
	}

	if len(p.Config.HistoryTypes) > 0 && len(historyPage.Records) == 0 {
		p.advanceHistory(nextHistoryID, notification.MessageID, "watch: failed to update state: %v")

		return nil, ErrNoNewMessages
	}

	historyIDs := CollectHistoryMessageIDs(historyPage.Records)

	batch, err := source.FetchMessages(ctx, historyIDs.FetchIDs)
	if err != nil {
		return nil, p.openRateLimitCircuitIfNeeded(err)
	}

	p.advanceHistory(nextHistoryID, notification.MessageID, "watch: failed to update state: %v")

	if batch.Excluded > 0 && len(batch.Messages) == 0 {
		if p.Config.Verbose {
			p.logf("watch: skipping hook; all messages excluded")
		}

		return nil, ErrNoNewMessages
	}

	return &Payload{
		Source:            "gmail",
		Account:           p.Config.Account,
		HistoryID:         nextHistoryID,
		Messages:          batch.Messages,
		DeletedMessageIDs: historyIDs.DeletedIDs,
	}, nil
}

func (p *Processor) resync(ctx context.Context, source Source, notification Notification) (*Payload, error) {
	ids, err := source.ListRecentMessageIDs(ctx, p.Config.ResyncMax)
	if err != nil {
		return nil, p.openRateLimitCircuitIfNeeded(err)
	}

	batch, err := source.FetchMessages(ctx, ids)
	if err != nil {
		return nil, p.openRateLimitCircuitIfNeeded(err)
	}

	p.advanceHistory(notification.HistoryID, notification.MessageID, "watch: failed to update state after resync: %v")

	if batch.Excluded > 0 && len(batch.Messages) == 0 {
		if p.Config.Verbose {
			p.logf("watch: skipping hook; all messages excluded")
		}

		return nil, ErrNoNewMessages
	}

	return &Payload{
		Source:    "gmail",
		Account:   p.Config.Account,
		HistoryID: notification.HistoryID,
		Messages:  batch.Messages,
	}, nil
}

func (p *Processor) advanceHistory(historyID, messageID, warning string) {
	if err := p.Repository.AdvanceHistory(historyID, messageID, p.currentTime()); err != nil {
		p.warnf(warning, err)
	}
}

func (p *Processor) logStaleNotification(historyID string) {
	if historyID == "" {
		return
	}

	state := p.Repository.Get()

	stale, err := IsStaleHistoryID(state.HistoryID, historyID)
	if err != nil {
		p.warnf("watch: history id compare failed: %v", err)

		return
	}

	if stale {
		p.logf("watch: ignoring stale push historyId=%s (stored=%s)", historyID, state.HistoryID)
	}
}

func (p *Processor) sleepForFetch(ctx context.Context) error {
	if p.Config.FetchDelay <= 0 {
		return nil
	}

	if p.Sleep != nil {
		return p.Sleep(ctx, p.Config.FetchDelay)
	}

	timer := time.NewTimer(p.Config.FetchDelay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return fmt.Errorf("wait for history fetch delay: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (p *Processor) checkRateLimitCircuit(now time.Time) error {
	until, open, err := p.Repository.CheckRateLimit(now)
	if err != nil {
		return err
	}

	if open {
		return &RateLimitError{Until: until}
	}

	return nil
}

func (p *Processor) openRateLimitCircuitIfNeeded(err error) error {
	if p.RateLimitUntil == nil {
		return err
	}

	now := p.currentTime()

	until, ok := p.RateLimitUntil(err, now)
	if !ok {
		return err
	}

	if updateErr := p.Repository.OpenRateLimit(until, err.Error(), now); updateErr != nil {
		p.warnf("watch: failed to update rate limit state: %v", updateErr)
	}

	return &RateLimitError{Until: until, Cause: err}
}

func (p *Processor) currentTime() time.Time {
	if p.Now != nil {
		return p.Now()
	}

	return time.Now()
}

func (p *Processor) logf(format string, args ...any) {
	if p.Logf != nil {
		p.Logf(format, args...)
	}
}

func (p *Processor) warnf(format string, args ...any) {
	if p.Warnf != nil {
		p.Warnf(format, args...)
	}
}
