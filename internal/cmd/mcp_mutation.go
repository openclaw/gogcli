package cmd

import (
	"context"
	"sync"

	"github.com/openclaw/gogcli/internal/googleapi"
)

type mcpMutationReceipt struct {
	Outcome         string   `json:"outcome"`
	KnownSteps      int      `json:"known_steps"`
	AttemptedSteps  int      `json:"attempted_steps"`
	IDs             []string `json:"ids,omitempty"`
	RetrySafe       bool     `json:"retry_safe"`
	MetadataOmitted bool     `json:"metadata_omitted"`
	DryRun          bool     `json:"dry_run,omitempty"`
}
type mcpMutationRecorder struct {
	mu                          sync.Mutex
	attempts, committed, failed int
	expected                    int
	unknown                     bool
	ids                         []string
	omitted                     bool
}
type mcpMutationKey struct{}

func (r *mcpMutationRecorder) observe(attempt googleapi.MutationAttempt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts++
	switch {
	case attempt.Unknown:
		r.unknown = true
	case attempt.StatusCode >= 200 && attempt.StatusCode < 300:
		r.committed++
	default:
		r.failed++
	}
}

func recordMCPMutationID(ctx context.Context, id string) {
	recorder, _ := ctx.Value(mcpMutationKey{}).(*mcpMutationRecorder)
	if recorder == nil || id == "" {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(id) > 256 || len(recorder.ids) >= 16 {
		recorder.omitted = true
		return
	}
	recorder.ids = append(recorder.ids, id)
}

func mcpMutationFromContext(ctx context.Context) *mcpMutationRecorder {
	recorder, _ := ctx.Value(mcpMutationKey{}).(*mcpMutationRecorder)
	return recorder
}

func (r *mcpMutationRecorder) receipt(err error, dryRun bool) mcpMutationReceipt {
	r.mu.Lock()
	defer r.mu.Unlock()
	outcome := "failed"
	switch {
	case dryRun || (r.attempts == 0 && err == nil):
		outcome = "not_attempted"
	case r.unknown:
		outcome = "outcome_unknown"
	case r.committed > 0 && (r.failed > 0 || (err != nil && r.expected > r.committed)):
		outcome = "partial"
	case r.committed > 0:
		outcome = "committed"
	}
	return mcpMutationReceipt{Outcome: outcome, KnownSteps: r.committed, AttemptedSteps: r.attempts, IDs: append([]string(nil), r.ids...), MetadataOmitted: r.omitted || (err != nil && r.committed > 0), DryRun: dryRun}
}

func mcpExecutionContext(ctx context.Context, flags *RootFlags, receipt *mcpMutationRecorder) context.Context {
	if flags.MCPBoundedRead {
		ctx = googleapi.WithResponseByteLimit(ctx, 12<<20)
	}
	if receipt != nil {
		ctx = context.WithValue(ctx, mcpMutationKey{}, receipt)
		ctx = googleapi.WithMutationObserver(ctx, receipt.observe)
		ctx = googleapi.WithoutRetries(ctx)
		ctx = googleapi.WithResponseByteLimit(ctx, 12<<20)
	}
	return ctx
}

func expectMCPMutationWrites(ctx context.Context) {
	recorder := mcpMutationFromContext(ctx)
	if recorder == nil {
		return
	}
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.expected = max(recorder.expected, recorder.attempts+2)
}
