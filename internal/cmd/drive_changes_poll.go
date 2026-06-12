package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"

	"github.com/steipete/gogcli/internal/outfmt"
)

const (
	driveChangesPollStateKind  = "drive_changes_poll"
	driveChangesServeStateKind = "drive_changes_serve"
)

type DriveChangesPollCmd struct {
	StateFile      string        `name:"state-file" required:"" help:"JSON file that stores the current Drive page token"`
	Interval       time.Duration `name:"interval" help:"Delay between polls" default:"60s"`
	OnChange       string        `name:"on-change" help:"Trusted local shell command run for each non-empty batch; batch JSON is provided on stdin"`
	FilterFile     string        `name:"filter-file" help:"Only emit and invoke the hook for changes to this file ID"`
	DriveID        string        `name:"drive" aliases:"drive-id" help:"Shared drive ID for a shared-drive change log"`
	MaxIterations  int           `name:"max-iterations" help:"Stop after N polls; 0 runs until interrupted" default:"0"`
	Max            int64         `name:"max" aliases:"limit" help:"Max changes per API page" default:"100"`
	IncludeRemoved bool          `name:"include-removed" help:"Include removed changes" default:"true" negatable:"_"`
}

type driveChangesPollState struct {
	Version   int    `json:"version"`
	Kind      string `json:"kind,omitempty"`
	PageToken string `json:"page_token"`
	DriveID   string `json:"drive_id,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

type driveChangesPollEvent struct {
	Kind          string          `json:"kind"`
	DriveID       string          `json:"driveId,omitempty"`
	PageToken     string          `json:"pageToken"`
	NextPageToken string          `json:"nextPageToken"`
	Changes       []*drive.Change `json:"changes"`
}

func (c *DriveChangesPollCmd) Run(ctx context.Context, flags *RootFlags) error {
	pollCtx, stop := pollSignalContext(ctx)
	defer stop()
	return c.run(pollCtx, flags, defaultPollRuntime())
}

func (c *DriveChangesPollCmd) run(ctx context.Context, flags *RootFlags, runtime pollRuntime) error {
	runtime = runtime.withDefaults()
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

	driveID := strings.TrimSpace(c.DriveID)
	filterFile := normalizeGoogleID(strings.TrimSpace(c.FilterFile))
	if dryRunErr := dryRunExit(ctx, flags, "drive.changes.poll", map[string]any{
		"state_file":      statePath,
		"interval":        c.Interval.String(),
		"drive_id":        driveID,
		"filter_file":     filterFile,
		"max_iterations":  c.MaxIterations,
		"max":             c.Max,
		"include_removed": c.IncludeRemoved,
		"hook_configured": strings.TrimSpace(c.OnChange) != "",
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}

	state, exists, err := readDriveChangesPollState(statePath)
	if err != nil {
		return err
	}
	if exists {
		if driveID != "" && driveID != state.DriveID {
			return usagef("poll state drive_id %q does not match --drive %q", state.DriveID, driveID)
		}
		if driveID == "" {
			driveID = state.DriveID
		}
	} else {
		startPageToken, startErr := getDriveChangesStartToken(ctx, svc, driveID)
		if startErr != nil {
			return startErr
		}
		state = driveChangesPollState{
			Version:   pollStateVersion,
			Kind:      driveChangesPollStateKind,
			PageToken: startPageToken,
			DriveID:   driveID,
			UpdatedAt: runtime.now().UTC().Format(time.RFC3339Nano),
		}
		if err := writePollState(statePath, state); err != nil {
			return err
		}
	}

	for iteration := 1; ; iteration++ {
		changes, nextPageToken, loadErr := loadDriveChanges(ctx, svc, state.PageToken, driveChangesLoadOptions{
			max:            c.Max,
			includeRemoved: c.IncludeRemoved,
			driveID:        driveID,
			all:            true,
		})
		if loadErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return loadErr
		}
		filtered := filterDriveChangesByFile(changes, filterFile)
		if len(filtered) > 0 {
			event := driveChangesPollEvent{
				Kind:          "drive_changes",
				DriveID:       driveID,
				PageToken:     state.PageToken,
				NextPageToken: nextPageToken,
				Changes:       filtered,
			}
			if err := writeDriveChangesPollEvent(ctx, event); err != nil {
				return err
			}
			if err := runtime.runHook(ctx, c.OnChange, event); err != nil {
				return err
			}
		}

		nextState := driveChangesPollState{
			Version:   pollStateVersion,
			Kind:      driveChangesPollStateKind,
			PageToken: nextPageToken,
			DriveID:   driveID,
			UpdatedAt: runtime.now().UTC().Format(time.RFC3339Nano),
		}
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

func readDriveChangesPollState(path string) (driveChangesPollState, bool, error) {
	var state driveChangesPollState
	exists, err := readPollState(path, &state)
	if err != nil || !exists {
		return state, exists, err
	}
	if state.Version != pollStateVersion {
		return driveChangesPollState{}, false, fmt.Errorf("unsupported Drive changes poll state version %d", state.Version)
	}
	switch state.Kind {
	case "", driveChangesPollStateKind:
		state.Kind = driveChangesPollStateKind
	case driveChangesServeStateKind:
		return driveChangesPollState{}, false, errors.New("state file belongs to drive changes serve; use a separate --state-file")
	default:
		return driveChangesPollState{}, false, fmt.Errorf("unsupported Drive changes poll state kind %q", state.Kind)
	}
	if strings.TrimSpace(state.PageToken) == "" {
		return driveChangesPollState{}, false, fmt.Errorf("drive changes poll state has empty page_token")
	}
	state.PageToken = strings.TrimSpace(state.PageToken)
	state.DriveID = strings.TrimSpace(state.DriveID)
	return state, true, nil
}

func filterDriveChangesByFile(changes []*drive.Change, fileID string) []*drive.Change {
	if strings.TrimSpace(fileID) == "" {
		return changes
	}
	filtered := make([]*drive.Change, 0, len(changes))
	for _, change := range changes {
		if change != nil && change.FileId == fileID {
			filtered = append(filtered, change)
		}
	}
	return filtered
}

func writeDriveChangesPollEvent(ctx context.Context, event driveChangesPollEvent) error {
	if outfmt.IsJSON(ctx) {
		return writePollJSON(ctx, event)
	}
	for _, change := range event.Changes {
		if change == nil {
			continue
		}
		name := ""
		if change.File != nil {
			name = change.File.Name
		}
		if _, err := fmt.Fprintf(
			stdoutWriter(ctx),
			"change\t%s\t%s\t%s\t%s\t%t\n",
			change.Time,
			change.Type,
			change.FileId,
			sanitizeTab(name),
			change.Removed,
		); err != nil {
			return fmt.Errorf("write poll output: %w", err)
		}
	}
	return nil
}
