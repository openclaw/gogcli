//nolint:wsl_v5 // Checkpoint state transitions stay grouped for readability.
package gmailbackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/backup"
)

const (
	CheckpointPhaseConfigured       = "configured"
	CheckpointPhaseFlushed          = "flushed"
	CheckpointPhaseReused           = "reused"
	CheckpointPhasePromotionSkipped = "promotion-skipped"
	CheckpointPhasePromoted         = "promoted"
)

var (
	errUnexpectedCheckpointShard  = errors.New("gmail checkpoint contains an unexpected shard")
	errCheckpointRowCountMismatch = errors.New("gmail checkpoint row count mismatch")
)

type CheckpointEvent struct {
	Phase   string
	Reason  string
	RunID   string
	Done    int
	Total   int
	Parts   int
	Rows    int
	Shards  int
	Changed bool
	Every   time.Duration
}

type CheckpointOptions struct {
	Enabled          bool
	AccountHash      string
	Query            string
	Max              int64
	IncludeSpamTrash bool
	RunID            string
	Rows             int
	Every            time.Duration
	Cache            MessageCache
	BackupOptions    backup.Options
	Now              func() time.Time
	Progress         func(CheckpointEvent)
}

type Checkpointer struct {
	enabled bool
	opts    CheckpointOptions
	total   int
	part    int
	last    time.Time
	pending []string
}

func NewCheckpointer(opts CheckpointOptions, total int) *Checkpointer {
	opts = normalizeCheckpointOptions(opts)
	enabled := opts.Enabled &&
		opts.Cache != nil &&
		strings.TrimSpace(opts.AccountHash) != "" &&
		strings.TrimSpace(opts.RunID) != "" &&
		(opts.Rows > 0 || opts.Every > 0)
	checkpointer := &Checkpointer{
		enabled: enabled,
		opts:    opts,
		total:   total,
		last:    opts.Now(),
	}
	if enabled {
		emitCheckpointEvent(opts.Progress, CheckpointEvent{
			Phase: CheckpointPhaseConfigured,
			RunID: opts.RunID,
			Rows:  opts.Rows,
			Every: opts.Every,
		})
	}
	return checkpointer
}

func (c *Checkpointer) Record(ctx context.Context, messageID string, event Event) error {
	if c == nil || !c.enabled || strings.TrimSpace(messageID) == "" {
		return nil
	}
	c.pending = append(c.pending, messageID)
	if !c.shouldFlush(event.Done) {
		return nil
	}
	return c.flush(ctx, event)
}

func (c *Checkpointer) shouldFlush(done int) bool {
	if len(c.pending) == 0 {
		return false
	}
	if c.opts.Rows > 0 && len(c.pending) >= c.opts.Rows {
		return true
	}
	if c.opts.Every > 0 && c.opts.Now().Sub(c.last) >= c.opts.Every {
		return true
	}
	return done == c.total
}

func (c *Checkpointer) flush(ctx context.Context, event Event) error {
	if c == nil || !c.enabled || len(c.pending) == 0 {
		return nil
	}
	ids := append([]string(nil), c.pending...)
	c.pending = c.pending[:0]
	firstPart := c.part + 1
	shards, err := BuildCheckpointShards(ctx, c.opts.Cache, ids, CheckpointShardOptions{
		AccountHash: c.opts.AccountHash,
		RunID:       c.opts.RunID,
		FirstPart:   firstPart,
	})
	if err != nil {
		return err
	}
	c.part += len(shards)
	snapshot := backup.Snapshot{
		Services: []string{Service},
		Accounts: []string{c.opts.AccountHash},
		Counts:   map[string]int{"gmail.messages": len(ids)},
		Shards:   shards,
	}
	result, err := backup.PushCheckpoint(ctx, snapshot, backup.Checkpoint{
		RunID:     c.opts.RunID,
		Service:   Service,
		Account:   c.opts.AccountHash,
		Done:      event.Done,
		Total:     c.total,
		Fetched:   event.Fetched,
		CacheHits: event.CacheHits,
	}, c.opts.BackupOptions)
	if err != nil {
		return fmt.Errorf("push Gmail backup checkpoint: %w", err)
	}
	c.last = c.opts.Now()
	emitCheckpointEvent(c.opts.Progress, CheckpointEvent{
		Phase:   CheckpointPhaseFlushed,
		RunID:   c.opts.RunID,
		Done:    event.Done,
		Total:   c.total,
		Parts:   len(shards),
		Rows:    len(ids),
		Changed: result.Changed,
	})
	return nil
}

func ResolveCheckpointRunID(ctx context.Context, ids []string, opts CheckpointOptions) string {
	opts = normalizeCheckpointOptions(opts)
	generated := opts.Now().UTC().Format("20060102T150405Z") + "-" + checkpointRunIDSuffix(opts, ids)
	if !opts.Enabled || strings.TrimSpace(opts.AccountHash) == "" {
		return generated
	}
	suffix := checkpointRunIDSuffix(opts, ids)
	cfg, err := backup.ResolveOptions(opts.BackupOptions)
	if err != nil {
		return generated
	}
	root := filepath.Join(cfg.Repo, "checkpoints", Service, opts.AccountHash)
	entries, err := os.ReadDir(root)
	if err != nil {
		return generated
	}
	runIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && strings.HasSuffix(entry.Name(), "-"+suffix) {
			runIDs = append(runIDs, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(runIDs)))
	for _, runID := range runIDs {
		if err := ctx.Err(); err != nil {
			return generated
		}
		manifest, err := backup.ReadCheckpointManifest(cfg.Repo, checkpointManifestRel(opts.AccountHash, runID))
		candidate := opts
		candidate.RunID = runID
		if err != nil || !checkpointManifestMatches(manifest, candidate, ids) {
			continue
		}
		emitCheckpointEvent(opts.Progress, CheckpointEvent{
			Phase: CheckpointPhaseReused,
			RunID: runID,
			Done:  manifest.Done,
			Total: manifest.Total,
		})
		return runID
	}
	return generated
}

func PromoteCheckpoint(ctx context.Context, ids []string, opts CheckpointOptions) ([]backup.PlainShard, bool, error) {
	if !opts.Enabled || strings.TrimSpace(opts.AccountHash) == "" || strings.TrimSpace(opts.RunID) == "" {
		return nil, false, nil
	}
	cfg, err := backup.ResolveOptions(opts.BackupOptions)
	if err != nil {
		return nil, false, fmt.Errorf("resolve Gmail backup checkpoint options: %w", err)
	}
	if len(cfg.Recipients) == 0 {
		recipient, recipientErr := backup.RecipientFromIdentity(cfg.Identity)
		if recipientErr != nil {
			return nil, false, fmt.Errorf("resolve Gmail backup checkpoint recipient: %w", recipientErr)
		}
		cfg.Recipients = []string{recipient}
	}
	manifest, err := backup.ReadCheckpointManifest(cfg.Repo, checkpointManifestRel(opts.AccountHash, opts.RunID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("read Gmail backup checkpoint manifest: %w", err)
	}
	if !checkpointManifestComplete(manifest, opts, ids) {
		return nil, false, nil
	}
	if !sameRecipients(manifest.Recipients, cfg.Recipients) {
		emitCheckpointEvent(opts.Progress, CheckpointEvent{
			Phase:  CheckpointPhasePromotionSkipped,
			Reason: "recipients-changed",
			RunID:  opts.RunID,
		})
		return nil, false, nil
	}
	shards := make([]backup.PlainShard, 0, len(manifest.Shards))
	rows := 0
	for _, entry := range manifest.Shards {
		if entry.Service != Service || entry.Kind != MessageShardKind || entry.Account != opts.AccountHash {
			return nil, false, fmt.Errorf(
				"%w: run %s has %s/%s/%s",
				errUnexpectedCheckpointShard,
				opts.RunID,
				entry.Service,
				entry.Kind,
				entry.Account,
			)
		}
		shards = append(shards, backup.ExistingShard(entry, manifest.Recipients))
		rows += entry.Rows
	}
	if rows != len(ids) {
		return nil, false, fmt.Errorf("%w: run %s has %d, want %d", errCheckpointRowCountMismatch, opts.RunID, rows, len(ids))
	}
	emitCheckpointEvent(opts.Progress, CheckpointEvent{
		Phase:  CheckpointPhasePromoted,
		RunID:  opts.RunID,
		Shards: len(shards),
		Rows:   rows,
	})
	return shards, true, nil
}

func normalizeCheckpointOptions(opts CheckpointOptions) CheckpointOptions {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func checkpointRunIDSuffix(opts CheckpointOptions, ids []string) string {
	key := struct {
		AccountHash      string `json:"account_hash"`
		Query            string `json:"query,omitempty"`
		Max              int64  `json:"max,omitempty"`
		IncludeSpamTrash bool   `json:"include_spam_trash"`
		MessageIDs       string `json:"message_ids"`
	}{
		AccountHash:      opts.AccountHash,
		Query:            strings.TrimSpace(opts.Query),
		Max:              opts.Max,
		IncludeSpamTrash: opts.IncludeSpamTrash,
		MessageIDs:       checkpointMessageIDsDigest(ids),
	}
	data, _ := json.Marshal(key)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:6])
}

func checkpointMessageIDsDigest(ids []string) string {
	hash := sha256.New()
	for _, id := range ids {
		_, _ = hash.Write([]byte(id))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func checkpointManifestRel(accountHash, runID string) string {
	return fmt.Sprintf("checkpoints/%s/%s/%s/manifest.json", Service, accountHash, runID)
}

func checkpointManifestMatches(manifest backup.CheckpointManifest, opts CheckpointOptions, ids []string) bool {
	return manifest.Service == Service &&
		manifest.Account == opts.AccountHash &&
		manifest.RunID == opts.RunID &&
		manifest.Total == len(ids) &&
		strings.TrimSpace(opts.RunID) != ""
}

func checkpointManifestComplete(manifest backup.CheckpointManifest, opts CheckpointOptions, ids []string) bool {
	return checkpointManifestMatches(manifest, opts, ids) &&
		manifest.Done == len(ids) &&
		manifest.Total == len(ids)
}

func sameRecipients(left, right []string) bool {
	left = normalizedStrings(left)
	right = normalizedStrings(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func normalizedStrings(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func emitCheckpointEvent(progress func(CheckpointEvent), event CheckpointEvent) {
	if progress != nil {
		progress(event)
	}
}
