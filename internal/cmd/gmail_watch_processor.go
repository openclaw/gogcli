package cmd

import (
	"context"

	"github.com/steipete/gogcli/internal/gmailwatch"
)

func (s *gmailWatchServer) watchProcessor() *gmailwatch.Processor {
	processor := &gmailwatch.Processor{
		Config: gmailwatch.ProcessorConfig{
			Account:      s.cfg.Account,
			HistoryMax:   s.cfg.HistoryMax,
			ResyncMax:    s.cfg.ResyncMax,
			FetchDelay:   s.cfg.FetchDelay,
			HistoryTypes: s.cfg.HistoryTypes,
			Verbose:      s.cfg.VerboseOutput,
		},
		Repository: s.store,
		NewSource: func(ctx context.Context) (gmailwatch.Source, error) {
			service, err := s.newService(ctx, s.cfg.Account)
			if err != nil {
				return nil, err
			}

			return newGmailWatchSource(service, s.cfg, s.excludeLabelIDs, s.logf), nil
		},
		Now:                 s.currentTime,
		Sleep:               s.sleep,
		IsStaleHistoryError: isStaleHistoryError,
		RateLimitUntil:      gmailWatchRateLimitUntil,
		Logf:                s.logf,
		Warnf:               s.warnf,
	}
	if s.cfg.HookURL != "" {
		processor.Deliver = s.deliverHook
	}

	return processor
}

func (s *gmailWatchServer) handlePush(ctx context.Context, payload gmailPushPayload) (*gmailHookPayload, error) {
	return s.watchProcessor().Handle(ctx, gmailwatch.Notification{
		HistoryID: payload.HistoryID,
		MessageID: payload.MessageID,
	})
}

func (s *gmailWatchServer) processGmailWatchPayload(ctx context.Context, payload gmailPushPayload) (*gmailWatchProcessedPayload, error) {
	return s.watchProcessor().Process(ctx, gmailwatch.Notification{
		HistoryID: payload.HistoryID,
		MessageID: payload.MessageID,
	})
}
