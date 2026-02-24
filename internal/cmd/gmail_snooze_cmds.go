package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/config"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/timeparse"
	"github.com/steipete/gogcli/internal/ui"
)

// GmailSnoozeCmd is the top-level snooze command group.
type GmailSnoozeCmd struct {
	Add       GmailSnoozeAddCmd       `cmd:"" name:"add" default:"withargs" aliases:"set" help:"Snooze a thread until a given time"`
	List      GmailSnoozeListCmd      `cmd:"" name:"list" aliases:"ls" help:"List pending snoozes"`
	Wake      GmailSnoozeWakeCmd      `cmd:"" name:"wake" aliases:"process" help:"Re-inbox threads whose snooze time has passed"`
	Cancel    GmailSnoozeCancelCmd    `cmd:"" name:"cancel" aliases:"unsnooze,remove" help:"Cancel a snooze and return thread to inbox"`
	Scheduler GmailSnoozeSchedulerCmd `cmd:"" name:"install-scheduler" aliases:"scheduler" help:"Install platform scheduler (launchd/systemd) to auto-wake snoozes"`
}

// GmailSnoozeAddCmd snoozes a thread until a given time.
type GmailSnoozeAddCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID to snooze"`
	Until    string `name:"until" short:"u" required:"" help:"Snooze until: RFC3339, YYYY-MM-DD, 'tomorrow 9am', 'friday', '2h', 'in 30 minutes'"`
}

func (c *GmailSnoozeAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	threadID := strings.TrimSpace(c.ThreadID)
	threadID = normalizeGmailThreadID(threadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	now := time.Now()
	wakeAt, err := parseUntilExpr(c.Until, now)
	if err != nil {
		return err
	}

	if !wakeAt.After(now) {
		return usagef("--until value must be in the future (got %s)", wakeAt.Format(time.RFC3339))
	}

	err = dryRunExit(ctx, flags, "gmail.snooze", map[string]any{
		"thread_id": threadID,
		"until":     wakeAt.Format(time.RFC3339),
	})
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	labelID, err := ensureSnoozedLabel(ctx, svc)
	if err != nil {
		return err
	}

	subject, err := fetchThreadSubject(ctx, svc, threadID)
	if err != nil {
		return err
	}

	err = snoozeApply(ctx, svc, threadID, labelID)
	if err != nil {
		return err
	}

	store, err := loadSnoozeStore(account)
	if err != nil {
		return err
	}

	entry := snoozeEntry{
		ThreadID:       threadID,
		Subject:        subject,
		Account:        account,
		UntilMs:        wakeAt.UnixMilli(),
		SnoozedLabelID: labelID,
		CreatedAtMs:    now.UnixMilli(),
	}
	if err := store.Upsert(entry); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"snoozed":  true,
			"threadId": threadID,
			"until":    wakeAt.Format(time.RFC3339),
			"subject":  subject,
		})
	}

	if u != nil {
		u.Out().Printf("Snoozed thread %s until %s", threadID, wakeAt.Format(time.RFC3339))
		if subject != "" {
			u.Out().Printf("Subject: %s", subject)
		}
	}
	return nil
}

// parseUntilExpr parses the --until flag value into a concrete time.Time.
// Strategy: try ParseRangeExpr first (handles RFC3339, dates, weekdays, today/tomorrow);
// if that fails, try time.ParseDuration for forms like "2h" or "30m" and add to now.
func parseUntilExpr(until string, now time.Time) (time.Time, error) {
	until = strings.TrimSpace(until)
	if until == "" {
		return time.Time{}, usage("--until is required")
	}

	// Try natural-language / date forms first.
	if t, err := timeparse.ParseRangeExpr(until, now, time.Local); err == nil {
		return t, nil
	}

	// Try plain Go duration (e.g. "2h", "30m", "in 30m" — strip "in " prefix).
	durationStr := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(until), "in "))
	if d, err := time.ParseDuration(durationStr); err == nil {
		return now.Add(d), nil
	}

	return time.Time{}, fmt.Errorf("cannot parse --until %q: try RFC3339, YYYY-MM-DD, 'tomorrow', 'friday', '2h', 'in 30 minutes'", until)
}

// GmailSnoozeListCmd lists pending snooze entries.
type GmailSnoozeListCmd struct {
	All      bool   `name:"all" help:"Show snoozes across all accounts"`
	Timezone string `name:"timezone" short:"z" help:"Output timezone (IANA name)"`
	Local    bool   `name:"local" help:"Use local timezone (default)"`
}

func (c *GmailSnoozeListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)

	loc, err := resolveOutputLocation(c.Timezone, c.Local)
	if err != nil {
		return err
	}

	entries, err := loadAllSnoozeEntries(c.All, flags)
	if err != nil {
		return err
	}

	// Sort by wake time ascending.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].UntilMs < entries[j].UntilMs
	})

	if outfmt.IsJSON(ctx) {
		type row struct {
			ThreadID  string `json:"threadId"`
			Subject   string `json:"subject"`
			Account   string `json:"account"`
			Until     string `json:"until"`
			CreatedAt string `json:"createdAt"`
		}
		rows := make([]row, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, row{
				ThreadID:  e.ThreadID,
				Subject:   e.Subject,
				Account:   e.Account,
				Until:     time.UnixMilli(e.UntilMs).In(loc).Format(time.RFC3339),
				CreatedAt: time.UnixMilli(e.CreatedAtMs).In(loc).Format(time.RFC3339),
			})
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"snoozes": rows})
	}

	if len(entries) == 0 {
		if u != nil {
			u.Out().Println("No pending snoozes")
		}
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "THREAD_ID\tSUBJECT\tACCOUNT\tUNTIL")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			e.ThreadID,
			e.Subject,
			e.Account,
			time.UnixMilli(e.UntilMs).In(loc).Format(time.RFC3339),
		)
	}
	return nil
}

// GmailSnoozeWakeCmd restores threads whose snooze time has passed back to INBOX.
type GmailSnoozeWakeCmd struct {
	All bool `name:"all" help:"Process all accounts (not just --account)"`
}

func (c *GmailSnoozeWakeCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	now := time.Now()
	isDryRun := flags != nil && flags.DryRun

	// Collect all state files to scan.
	storePaths, err := collectSnoozeStorePaths(c.All, flags)
	if err != nil {
		return err
	}

	type wokeResult struct {
		ThreadID string `json:"threadId"`
		Subject  string `json:"subject"`
		Account  string `json:"account"`
		WokeAt   string `json:"wokeAt"`
	}

	var woke []wokeResult

	for _, storePath := range storePaths {
		store, err := loadSnoozeStoreFromPath(storePath)
		if err != nil {
			return err
		}
		due := store.DueEntries(now, "")
		for _, entry := range due {
			if isDryRun {
				if u != nil {
					u.Out().Printf("Would wake thread %s (%s) for account %s", entry.ThreadID, entry.Subject, entry.Account)
				}
				woke = append(woke, wokeResult{
					ThreadID: entry.ThreadID,
					Subject:  entry.Subject,
					Account:  entry.Account,
					WokeAt:   now.Format(time.RFC3339),
				})
				continue
			}

			svc, err := newGmailService(ctx, entry.Account)
			if err != nil {
				return fmt.Errorf("account %s: %w", entry.Account, err)
			}
			if err := snoozeRestore(ctx, svc, entry.ThreadID, entry.SnoozedLabelID); err != nil {
				return fmt.Errorf("restore thread %s: %w", entry.ThreadID, err)
			}
			if err := store.Remove(entry.ThreadID, entry.Account); err != nil {
				return fmt.Errorf("remove snooze state for %s: %w", entry.ThreadID, err)
			}
			if u != nil {
				u.Out().Printf("Woke thread %s (%s) for account %s", entry.ThreadID, entry.Subject, entry.Account)
			}
			woke = append(woke, wokeResult{
				ThreadID: entry.ThreadID,
				Subject:  entry.Subject,
				Account:  entry.Account,
				WokeAt:   now.Format(time.RFC3339),
			})
		}
	}

	if outfmt.IsJSON(ctx) {
		if woke == nil {
			woke = []wokeResult{}
		}
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"woke":   woke,
			"dryRun": isDryRun,
		})
	}

	if len(woke) == 0 && u != nil {
		u.Out().Println("No snoozes due")
	}
	return nil
}

// GmailSnoozeCancelCmd cancels a snooze and returns the thread to INBOX.
type GmailSnoozeCancelCmd struct {
	ThreadID string `arg:"" name:"threadId" help:"Thread ID to unsnooze"`
}

func (c *GmailSnoozeCancelCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	threadID := strings.TrimSpace(c.ThreadID)
	threadID = normalizeGmailThreadID(threadID)
	if threadID == "" {
		return usage("empty threadId")
	}

	store, err := loadSnoozeStore(account)
	if err != nil {
		return err
	}

	// Find the entry to get the snoozed label ID.
	var found *snoozeEntry
	for i := range store.state.Entries {
		e := &store.state.Entries[i]
		if e.ThreadID == threadID && e.Account == account {
			found = e
			break
		}
	}
	if found == nil {
		return fmt.Errorf("snooze not found for thread %s", threadID)
	}
	subject := found.Subject
	labelID := found.SnoozedLabelID

	err = dryRunExit(ctx, flags, "gmail.snooze.cancel", map[string]any{
		"thread_id": threadID,
	})
	if err != nil {
		return err
	}

	svc, err := newGmailService(ctx, account)
	if err != nil {
		return err
	}

	err = snoozeRestore(ctx, svc, threadID, labelID)
	if err != nil {
		return err
	}

	err = store.Remove(threadID, account)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"cancelled": true,
			"threadId":  threadID,
			"subject":   subject,
		})
	}

	if u := ui.FromContext(ctx); u != nil {
		u.Out().Printf("Cancelled snooze for thread %s", threadID)
		if subject != "" {
			u.Out().Printf("Subject: %s", subject)
		}
	}
	return nil
}

// loadAllSnoozeEntries loads entries from one account (if all==false) or all
// state files in the snooze dir (if all==true).
func loadAllSnoozeEntries(all bool, flags *RootFlags) ([]snoozeEntry, error) {
	if !all {
		account, err := requireAccount(flags)
		if err != nil {
			return nil, err
		}
		store, err := loadSnoozeStore(account)
		if err != nil {
			return nil, err
		}
		return store.AllEntries(account), nil
	}

	paths, err := collectSnoozeStorePaths(true, flags)
	if err != nil {
		return nil, err
	}
	var out []snoozeEntry
	for _, p := range paths {
		store, err := loadSnoozeStoreFromPath(p)
		if err != nil {
			return nil, err
		}
		out = append(out, store.AllEntries("")...)
	}
	return out, nil
}

// collectSnoozeStorePaths returns paths to snooze state files. When all==true,
// it globs the snooze dir for all *.json files. Otherwise it returns the single
// path for flags.Account.
func collectSnoozeStorePaths(all bool, flags *RootFlags) ([]string, error) {
	if !all {
		account, err := requireAccount(flags)
		if err != nil {
			return nil, err
		}
		p, err := gmailSnoozeStatePath(account)
		if err != nil {
			return nil, err
		}
		return []string{p}, nil
	}

	dir, err := config.GmailSnoozeDir()
	if err != nil {
		return nil, err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// loadSnoozeStoreFromPath loads a snooze store directly from a file path,
// bypassing the account-to-path resolution. Used when scanning all accounts.
func loadSnoozeStoreFromPath(path string) (*snoozeStore, error) {
	store := &snoozeStore{path: path}
	data, err := os.ReadFile(path) //nolint:gosec // path comes from controlled glob of config directory
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &store.state); err != nil {
		return nil, err
	}
	return store, nil
}

// GmailSnoozeSchedulerCmd is defined in gmail_snooze_scheduler.go.
