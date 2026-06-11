package cmd

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
)

type DocsCommentsPollCmd struct {
	DocID           string        `arg:"" name:"docId" help:"Google Doc ID or URL"`
	StateFile       string        `name:"state-file" required:"" help:"JSON file that stores the comment time watermark"`
	Interval        time.Duration `name:"interval" help:"Delay between polls" default:"60s"`
	IncludeResolved bool          `name:"include-resolved" aliases:"resolved" help:"Include resolved comments"`
	OnNew           string        `name:"on-new" help:"Trusted local shell command run for each comment; comment event JSON is provided on stdin"`
	MaxIterations   int           `name:"max-iterations" help:"Stop after N polls; 0 runs until interrupted" default:"0"`
	Max             int64         `name:"max" aliases:"limit" help:"Max comments per API page" default:"100"`
}

type docsCommentsPollState struct {
	Version         int      `json:"version"`
	DocID           string   `json:"doc_id"`
	Watermark       string   `json:"watermark"`
	SeenIDs         []string `json:"seen_ids,omitempty"`
	IncludeResolved bool     `json:"include_resolved"`
	UpdatedAt       string   `json:"updated_at"`
}

type docsCommentPollEvent struct {
	Kind    string         `json:"kind"`
	DocID   string         `json:"docId"`
	Comment *drive.Comment `json:"comment"`
}

type timedDriveComment struct {
	comment *drive.Comment
	at      time.Time
}

func (c *DocsCommentsPollCmd) Run(ctx context.Context, flags *RootFlags) error {
	pollCtx, stop := pollSignalContext(ctx)
	defer stop()
	return c.run(pollCtx, flags, defaultPollRuntime())
}

func (c *DocsCommentsPollCmd) run(ctx context.Context, flags *RootFlags, runtime pollRuntime) error {
	runtime = runtime.withDefaults()
	docID := normalizeGoogleID(strings.TrimSpace(c.DocID))
	if docID == "" {
		return usage("empty docId")
	}
	statePath, err := expandPollStatePath(c.StateFile)
	if err != nil {
		return err
	}
	if c.Interval <= 0 {
		return usage("--interval must be greater than zero")
	}
	if c.MaxIterations < 0 {
		return usage("--max-iterations must be >= 0")
	}
	if c.Max <= 0 {
		return usage("--max must be greater than zero")
	}

	if dryRunErr := dryRunExit(ctx, flags, "docs.comments.poll", map[string]any{
		"doc_id":           docID,
		"state_file":       statePath,
		"interval":         c.Interval.String(),
		"include_resolved": c.IncludeResolved,
		"max_iterations":   c.MaxIterations,
		"max":              c.Max,
		"hook_configured":  strings.TrimSpace(c.OnNew) != "",
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	state, exists, err := readDocsCommentsPollState(statePath)
	if err != nil {
		return err
	}
	if exists {
		if state.DocID != docID {
			return usagef("poll state doc_id %q does not match docId %q", state.DocID, docID)
		}
		if state.IncludeResolved != c.IncludeResolved {
			return usage("poll state --include-resolved setting does not match")
		}
	} else {
		now := runtime.now().UTC()
		state = docsCommentsPollState{
			Version:         pollStateVersion,
			DocID:           docID,
			Watermark:       now.Format(time.RFC3339Nano),
			IncludeResolved: c.IncludeResolved,
			UpdatedAt:       now.Format(time.RFC3339Nano),
		}
		if err := writePollState(statePath, state); err != nil {
			return err
		}
	}

	for iteration := 1; ; iteration++ {
		comments, _, listErr := listDriveComments(ctx, svc, docID, driveCommentListOptions{
			resourceKey:     "docId",
			resourceID:      docID,
			includeResolved: true,
			since:           state.Watermark,
			all:             true,
			max:             c.Max,
			mode:            driveCommentListModeExpanded,
		})
		if listErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return listErr
		}

		allFresh, filterErr := filterPolledDriveComments(comments, state)
		if filterErr != nil {
			return filterErr
		}
		fresh := filterPolledCommentsByResolved(allFresh, c.IncludeResolved)
		for _, item := range fresh {
			event := docsCommentPollEvent{
				Kind:    "docs_comment",
				DocID:   docID,
				Comment: item.comment,
			}
			if err := writeDocsCommentPollEvent(ctx, event, item.at); err != nil {
				return err
			}
			if err := runtime.runHook(ctx, c.OnNew, event); err != nil {
				return err
			}
		}

		nextState := advanceDocsCommentsPollState(state, allFresh, runtime.now().UTC())
		if err := writePollState(statePath, nextState); err != nil {
			return err
		}
		state = nextState

		if c.MaxIterations > 0 && iteration >= c.MaxIterations {
			return nil
		}
		if err := runtime.wait(ctx, c.Interval); err != nil {
			return err
		}
	}
}

func readDocsCommentsPollState(path string) (docsCommentsPollState, bool, error) {
	var state docsCommentsPollState
	exists, err := readPollState(path, &state)
	if err != nil || !exists {
		return state, exists, err
	}
	if state.Version != pollStateVersion {
		return docsCommentsPollState{}, false, fmt.Errorf("unsupported docs comments poll state version %d", state.Version)
	}
	state.DocID = strings.TrimSpace(state.DocID)
	if state.DocID == "" {
		return docsCommentsPollState{}, false, fmt.Errorf("docs comments poll state has empty doc_id")
	}
	watermark, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.Watermark))
	if err != nil {
		return docsCommentsPollState{}, false, fmt.Errorf("docs comments poll state has invalid watermark: %w", err)
	}
	state.Watermark = watermark.UTC().Format(time.RFC3339Nano)
	state.SeenIDs = normalizedPollIDs(state.SeenIDs)
	return state, true, nil
}

func filterPolledDriveComments(comments []*drive.Comment, state docsCommentsPollState) ([]timedDriveComment, error) {
	watermark, err := time.Parse(time.RFC3339Nano, state.Watermark)
	if err != nil {
		return nil, fmt.Errorf("parse docs comments poll watermark: %w", err)
	}
	seen := make(map[string]struct{}, len(state.SeenIDs))
	for _, id := range state.SeenIDs {
		seen[id] = struct{}{}
	}

	fresh := make([]timedDriveComment, 0, len(comments))
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		at, timeErr := driveCommentPollTime(comment)
		if timeErr != nil {
			return nil, timeErr
		}
		if at.Before(watermark) {
			continue
		}
		if at.Equal(watermark) {
			if _, ok := seen[comment.Id]; ok {
				continue
			}
		}
		fresh = append(fresh, timedDriveComment{comment: comment, at: at})
	}
	sort.SliceStable(fresh, func(i, j int) bool {
		if fresh[i].at.Equal(fresh[j].at) {
			return fresh[i].comment.Id < fresh[j].comment.Id
		}
		return fresh[i].at.Before(fresh[j].at)
	})
	return fresh, nil
}

func filterPolledCommentsByResolved(comments []timedDriveComment, includeResolved bool) []timedDriveComment {
	if includeResolved {
		return comments
	}
	filtered := make([]timedDriveComment, 0, len(comments))
	for _, item := range comments {
		if !item.comment.Resolved {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func driveCommentPollTime(comment *drive.Comment) (time.Time, error) {
	raw := strings.TrimSpace(comment.ModifiedTime)
	if raw == "" {
		raw = strings.TrimSpace(comment.CreatedTime)
	}
	if raw == "" {
		return time.Time{}, fmt.Errorf("drive comment %q has no modifiedTime or createdTime", comment.Id)
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("drive comment %q has invalid timestamp %q: %w", comment.Id, raw, err)
	}
	return at.UTC(), nil
}

func advanceDocsCommentsPollState(state docsCommentsPollState, fresh []timedDriveComment, updatedAt time.Time) docsCommentsPollState {
	watermark, _ := time.Parse(time.RFC3339Nano, state.Watermark)
	seen := make(map[string]struct{}, len(state.SeenIDs))
	for _, id := range state.SeenIDs {
		seen[id] = struct{}{}
	}
	for _, item := range fresh {
		switch {
		case item.at.After(watermark):
			watermark = item.at
			clear(seen)
			seen[item.comment.Id] = struct{}{}
		case item.at.Equal(watermark):
			seen[item.comment.Id] = struct{}{}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	state.Watermark = watermark.UTC().Format(time.RFC3339Nano)
	state.SeenIDs = ids
	state.UpdatedAt = updatedAt.Format(time.RFC3339Nano)
	return state
}

func normalizedPollIDs(ids []string) []string {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			unique[id] = struct{}{}
		}
	}
	normalized := make([]string, 0, len(unique))
	for id := range unique {
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}

func writeDocsCommentPollEvent(ctx context.Context, event docsCommentPollEvent, at time.Time) error {
	if outfmt.IsJSON(ctx) {
		return writePollJSON(ctx, event)
	}
	author := ""
	if event.Comment.Author != nil {
		author = event.Comment.Author.DisplayName
	}
	if _, err := fmt.Fprintf(
		os.Stdout,
		"comment\t%s\t%s\t%s\t%s\t%t\n",
		event.Comment.Id,
		oneLineTSV(author),
		oneLineTSV(event.Comment.Content),
		at.Format(time.RFC3339Nano),
		event.Comment.Resolved,
	); err != nil {
		return fmt.Errorf("write poll output: %w", err)
	}
	return nil
}
