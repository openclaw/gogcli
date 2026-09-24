package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/openclaw/gogcli/internal/docsbatch"
)

func batchSubmissionAllowed(flags *RootFlags, profile bakedSafetyProfile) bool {
	path := []string{"batch", "end"}
	if !profile.allowsCommandPath(path) {
		return false
	}
	if flags == nil {
		return true
	}
	if flags.ReadOnly || commandPathMatches(parseEnabledCommands(flags.DisableCommands), path) {
		return false
	}
	allow := parseEnabledCommands(flags.EnableCommands)
	exact := parseEnabledCommands(flags.EnableCommandsExact)
	return (len(allow) == 0 && len(exact) == 0) || commandPathMatches(allow, path) || commandPathMatchesExact(exact, path)
}

func validateBatchSubmission(flags *RootFlags, state *docsbatch.State) error {
	switch state.Service {
	case docsbatch.ServiceDocs:
		if strings.TrimSpace(state.DocumentID) == "" || state.PresentationID != "" {
			return usage("invalid stored Docs batch target")
		}
		return nil
	case docsbatch.ServiceSlides:
		if strings.TrimSpace(state.PresentationID) == "" || state.DocumentID != "" {
			return usage("invalid stored Slides batch target")
		}
		if strings.TrimSpace(state.Account) == "" || strings.TrimSpace(state.Client) == "" || strings.TrimSpace(state.RequiredRevisionID) == "" {
			return usage("Slides batch is missing its bound account, OAuth client, or revision")
		}
		return enforceExplicitCommandPermission(flags, []string{"slides", "batch-submit"})
	default:
		return usagef("unsupported stored batch service %q", state.Service)
	}
}

func batchWirePayload(state *docsbatch.State, entries []docsbatch.RequestEntry) any {
	if state.Service == docsbatch.ServiceSlides {
		return slidesBatchWirePayload(state, entries)
	}
	return docsBatchWirePayload(state, entries)
}

func submitPersistedBatch(ctx context.Context, state *docsbatch.State, entries []docsbatch.RequestEntry) (string, error) {
	switch state.Service {
	case docsbatch.ServiceDocs:
		result, err := submitDocsBatch(ctx, state, entries)
		return docsBatchResponseRevision(result), err
	case docsbatch.ServiceSlides:
		return submitSlidesBatch(ctx, state, entries)
	default:
		return "", fmt.Errorf("unsupported batch service %q", state.Service)
	}
}

func batchUsesRevisions(service string) bool {
	return service == docsbatch.ServiceDocs || service == docsbatch.ServiceSlides
}
