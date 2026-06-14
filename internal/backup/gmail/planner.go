//nolint:wsl_v5 // Dense partition loops keep related state transitions together.
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
	Service                            = "gmail"
	MessageShardKind                   = "messages"
	DefaultMessageShardMaxRows         = 1000
	DefaultCheckpointShardMaxRows      = 250
	DefaultMessageShardMaxPlaintext    = int64(32 * 1024 * 1024)
	DefaultCheckpointShardMaxPlaintext = int64(32 * 1024 * 1024)
)

var (
	errTempShardDirUnavailable       = errors.New("gmail backup temp shard directory unavailable")
	errCheckpointPartNotPositive     = errors.New("gmail checkpoint part must be positive")
	errCheckpointShardDirUnavailable = errors.New("gmail backup checkpoint temp shard directory unavailable")
	errMessageMissingFromBackupCache = errors.New("gmail message missing from backup cache")
)

type ShardEvent struct {
	Phase  string
	Done   int
	Total  int
	Shards int
}

type ShardOptions struct {
	AccountHash      string
	MaxRows          int
	MaxPlaintextSize int64
	Progress         func(ShardEvent)
}

type CheckpointShardOptions struct {
	AccountHash      string
	RunID            string
	FirstPart        int
	MaxRows          int
	MaxPlaintextSize int64
}

type MessageCache interface {
	ReadMessage(accountHash, messageID string) (Message, bool, error)
	MessageShardDir(accountHash string) (string, bool)
	CheckpointShardDir(accountHash, runID string) (string, bool)
}

type messageRef struct {
	ID           string
	InternalDate int64
	LineBytes    int64
}

func BuildMessageShards(ctx context.Context, cache MessageCache, ids []string, opts ShardOptions) ([]backup.PlainShard, error) {
	opts = normalizeShardOptions(opts)
	tempDir, ok := cache.MessageShardDir(opts.AccountHash)
	if !ok {
		return nil, errTempShardDirUnavailable
	}
	if err := os.RemoveAll(tempDir); err != nil {
		return nil, fmt.Errorf("clear gmail backup temp shard dir: %w", err)
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return nil, fmt.Errorf("create gmail backup temp shard dir: %w", err)
	}

	buckets := make(map[string][]messageRef)
	for i, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, cleanupShardDir(tempDir, fmt.Errorf("build Gmail message shards: %w", err))
		}
		msg, found, err := cache.ReadMessage(opts.AccountHash, id)
		if err != nil {
			return nil, cleanupShardDir(tempDir, fmt.Errorf("read gmail backup cache %s: %w", id, err))
		}
		if !found {
			return nil, cleanupShardDir(tempDir, fmt.Errorf("%w: %s", errMessageMissingFromBackupCache, id))
		}
		lineBytes, err := messageJSONLSize(msg)
		if err != nil {
			return nil, cleanupShardDir(tempDir, err)
		}
		key := messageMonthKey(msg.InternalDate)
		buckets[key] = append(buckets[key], messageRef{
			ID:           msg.ID,
			InternalDate: msg.InternalDate,
			LineBytes:    lineBytes,
		})
		done := i + 1
		if done == len(ids) || done%5000 == 0 {
			emitShardEvent(opts.Progress, ShardEvent{Phase: "index", Done: done, Total: len(ids)})
		}
	}

	keys := sortedBucketKeys(buckets)
	shards := make([]backup.PlainShard, 0, len(keys))
	rows := 0
	for _, key := range keys {
		refs := buckets[key]
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].InternalDate == refs[j].InternalDate {
				return refs[i].ID < refs[j].ID
			}
			return refs[i].InternalDate < refs[j].InternalDate
		})
		for part, start := 1, 0; start < len(refs); part++ {
			if err := ctx.Err(); err != nil {
				return nil, cleanupShardDir(tempDir, fmt.Errorf("build Gmail message shards: %w", err))
			}
			end := messageChunkEnd(refs, start, opts.MaxRows, opts.MaxPlaintextSize)
			rel := fmt.Sprintf("data/gmail/%s/messages/%s/part-%04d.jsonl.gz.age", opts.AccountHash, key, part)
			shard, err := buildMessageShardFromCache(cache, opts.AccountHash, rel, tempDir, refs[start:end])
			if err != nil {
				return nil, cleanupShardDir(tempDir, err)
			}
			shards = append(shards, shard)
			rows += shard.Rows
			start = end
			if len(shards)%25 == 0 || start == len(refs) {
				emitShardEvent(opts.Progress, ShardEvent{
					Phase:  "build",
					Done:   rows,
					Total:  len(ids),
					Shards: len(shards),
				})
			}
		}
	}
	return shards, nil
}

func BuildMessageShardsFromMessages(ctx context.Context, messages []Message, opts ShardOptions) ([]backup.PlainShard, error) {
	opts = normalizeShardOptions(opts)
	buckets := make(map[string][]Message)
	for _, message := range messages {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("build Gmail message shards: %w", err)
		}
		key := messageMonthKey(message.InternalDate)
		buckets[key] = append(buckets[key], message)
	}

	keys := sortedBucketKeys(buckets)
	shards := make([]backup.PlainShard, 0, len(keys))
	rows := 0
	for _, key := range keys {
		values := buckets[key]
		sort.Slice(values, func(i, j int) bool {
			if values[i].InternalDate == values[j].InternalDate {
				return values[i].ID < values[j].ID
			}
			return values[i].InternalDate < values[j].InternalDate
		})
		for part, start := 1, 0; start < len(values); part++ {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("build Gmail message shards: %w", err)
			}
			end, err := messageValueChunkEnd(values, start, opts.MaxRows, opts.MaxPlaintextSize)
			if err != nil {
				return nil, err
			}
			rel := fmt.Sprintf("data/gmail/%s/messages/%s/part-%04d.jsonl.gz.age", opts.AccountHash, key, part)
			shard, err := backup.NewJSONLShard(Service, MessageShardKind, opts.AccountHash, rel, values[start:end])
			if err != nil {
				return nil, fmt.Errorf("build Gmail message shard: %w", err)
			}
			shards = append(shards, shard)
			rows += shard.Rows
			start = end
			emitShardEvent(opts.Progress, ShardEvent{
				Phase:  "build",
				Done:   rows,
				Total:  len(messages),
				Shards: len(shards),
			})
		}
	}
	return shards, nil
}

func BuildCheckpointShards(ctx context.Context, cache MessageCache, ids []string, opts CheckpointShardOptions) ([]backup.PlainShard, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if opts.FirstPart <= 0 {
		return nil, errCheckpointPartNotPositive
	}
	if opts.MaxRows <= 0 {
		opts.MaxRows = DefaultCheckpointShardMaxRows
	}
	if opts.MaxPlaintextSize == 0 {
		opts.MaxPlaintextSize = DefaultCheckpointShardMaxPlaintext
	}

	shards := make([]backup.PlainShard, 0, (len(ids)+opts.MaxRows-1)/opts.MaxRows)
	chunk := make([]string, 0, opts.MaxRows)
	var chunkBytes int64
	cleanup := func(err error) ([]backup.PlainShard, error) {
		for _, shard := range shards {
			if strings.TrimSpace(shard.PlaintextPath) != "" {
				_ = os.Remove(shard.PlaintextPath)
			}
		}
		return nil, err
	}
	flush := func() error {
		if len(chunk) == 0 {
			return nil
		}
		shard, err := buildCheckpointShard(cache, opts, opts.FirstPart+len(shards), chunk)
		if err != nil {
			return err
		}
		shards = append(shards, shard)
		chunk = chunk[:0]
		chunkBytes = 0
		return nil
	}

	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return cleanup(fmt.Errorf("build Gmail checkpoint shards: %w", err))
		}
		msg, found, err := cache.ReadMessage(opts.AccountHash, id)
		if err != nil {
			return cleanup(fmt.Errorf("read gmail backup cache %s: %w", id, err))
		}
		if !found {
			return cleanup(fmt.Errorf("%w: %s", errMessageMissingFromBackupCache, id))
		}
		lineBytes, err := messageJSONLSize(msg)
		if err != nil {
			return cleanup(err)
		}
		overRows := len(chunk) >= opts.MaxRows
		overBytes := opts.MaxPlaintextSize > 0 && len(chunk) > 0 && chunkBytes+lineBytes > opts.MaxPlaintextSize
		if overRows || overBytes {
			if err := flush(); err != nil {
				return cleanup(err)
			}
		}
		chunk = append(chunk, id)
		chunkBytes += lineBytes
	}
	if err := flush(); err != nil {
		return cleanup(err)
	}
	return shards, nil
}

func normalizeShardOptions(opts ShardOptions) ShardOptions {
	if opts.MaxRows <= 0 {
		opts.MaxRows = DefaultMessageShardMaxRows
	}
	if opts.MaxPlaintextSize == 0 {
		opts.MaxPlaintextSize = DefaultMessageShardMaxPlaintext
	}
	return opts
}

func sortedBucketKeys[T any](buckets map[string][]T) []string {
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func messageChunkEnd(refs []messageRef, start, maxRows int, maxBytes int64) int {
	end := start
	var chunkBytes int64
	for end < len(refs) {
		lineBytes := refs[end].LineBytes
		overRows := end-start >= maxRows
		overBytes := maxBytes > 0 && end > start && chunkBytes+lineBytes > maxBytes
		if overRows || overBytes {
			break
		}
		chunkBytes += lineBytes
		end++
	}
	return end
}

func messageValueChunkEnd(messages []Message, start, maxRows int, maxBytes int64) (int, error) {
	end := start
	var chunkBytes int64
	for end < len(messages) {
		lineBytes, err := messageJSONLSize(messages[end])
		if err != nil {
			return 0, err
		}
		overRows := end-start >= maxRows
		overBytes := maxBytes > 0 && end > start && chunkBytes+lineBytes > maxBytes
		if overRows || overBytes {
			break
		}
		chunkBytes += lineBytes
		end++
	}
	return end, nil
}

func buildMessageShardFromCache(cache MessageCache, accountHash, rel, tempDir string, refs []messageRef) (backup.PlainShard, error) {
	sum := sha256.Sum256([]byte(rel))
	path := filepath.Join(tempDir, hex.EncodeToString(sum[:])+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // path is rooted in the injected cache and uses a hash of the shard path.
	if err != nil {
		return backup.PlainShard{}, fmt.Errorf("create gmail backup temp shard: %w", err)
	}
	if chmodErr := f.Chmod(0o600); chmodErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("chmod gmail backup temp shard: %w", chmodErr)
	}
	count, err := encodeCachedMessages(f, cache, accountHash, refs)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return backup.PlainShard{}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("close gmail backup temp shard: %w", err)
	}
	return backup.PlainShard{
		Service:       Service,
		Kind:          MessageShardKind,
		Account:       accountHash,
		Path:          filepath.ToSlash(rel),
		Rows:          count,
		PlaintextPath: path,
	}, nil
}

func buildCheckpointShard(cache MessageCache, opts CheckpointShardOptions, part int, ids []string) (backup.PlainShard, error) {
	tempDir, ok := cache.CheckpointShardDir(opts.AccountHash, opts.RunID)
	if !ok {
		return backup.PlainShard{}, errCheckpointShardDirUnavailable
	}
	if err := os.MkdirAll(tempDir, 0o700); err != nil {
		return backup.PlainShard{}, fmt.Errorf("create gmail backup checkpoint temp shard dir: %w", err)
	}
	rel := fmt.Sprintf("checkpoints/gmail/%s/%s/messages/part-%06d.jsonl.gz.age", opts.AccountHash, opts.RunID, part)
	sum := sha256.Sum256([]byte(rel))
	path := filepath.Join(tempDir, hex.EncodeToString(sum[:])+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // path is rooted in the injected cache and uses a hash of the checkpoint path.
	if err != nil {
		return backup.PlainShard{}, fmt.Errorf("create gmail backup checkpoint temp shard: %w", err)
	}
	if chmodErr := f.Chmod(0o600); chmodErr != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("chmod gmail backup checkpoint shard: %w", chmodErr)
	}
	refs := make([]messageRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, messageRef{ID: id})
	}
	count, err := encodeCachedMessages(f, cache, opts.AccountHash, refs)
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return backup.PlainShard{}, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return backup.PlainShard{}, fmt.Errorf("close gmail backup checkpoint shard: %w", err)
	}
	return backup.PlainShard{
		Service:       Service,
		Kind:          MessageShardKind,
		Account:       opts.AccountHash,
		Path:          filepath.ToSlash(rel),
		Rows:          count,
		PlaintextPath: path,
	}, nil
}

func encodeCachedMessages(f *os.File, cache MessageCache, accountHash string, refs []messageRef) (int, error) {
	enc := json.NewEncoder(f)
	count := 0
	for _, ref := range refs {
		msg, found, err := cache.ReadMessage(accountHash, ref.ID)
		if err != nil {
			return 0, fmt.Errorf("read gmail backup cache %s: %w", ref.ID, err)
		}
		if !found {
			return 0, fmt.Errorf("%w: %s", errMessageMissingFromBackupCache, ref.ID)
		}
		if err := enc.Encode(msg); err != nil {
			return 0, fmt.Errorf("encode gmail backup temp shard: %w", err)
		}
		count++
	}
	return count, nil
}

func messageMonthKey(internalDate int64) string {
	t := time.UnixMilli(internalDate).UTC()
	if internalDate <= 0 {
		t = time.Unix(0, 0).UTC()
	}
	return fmt.Sprintf("%04d/%02d", t.Year(), int(t.Month()))
}

func messageJSONLSize(message Message) (int64, error) {
	line, err := json.Marshal(message)
	if err != nil {
		return 0, fmt.Errorf("encode gmail backup shard estimate: %w", err)
	}
	return int64(len(line) + 1), nil
}

func cleanupShardDir(path string, cause error) error {
	if err := os.RemoveAll(path); err != nil {
		return errors.Join(cause, fmt.Errorf("cleanup gmail backup temp shard dir: %w", err))
	}
	return cause
}

func emitShardEvent(progress func(ShardEvent), event ShardEvent) {
	if progress != nil {
		progress(event)
	}
}
