//nolint:err113,govet,revive,wrapcheck,wsl_v5 // Contextual errors keep backup call sites readable.
package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
)

const formatVersion = 1

type Manifest struct {
	Format     int            `json:"format"`
	App        string         `json:"app"`
	Encrypted  bool           `json:"encrypted"`
	Exported   time.Time      `json:"exported"`
	Recipients []string       `json:"recipients,omitempty"`
	Services   []string       `json:"services,omitempty"`
	Accounts   []string       `json:"accounts,omitempty"`
	Counts     map[string]int `json:"counts,omitempty"`
	Shards     []ShardEntry   `json:"shards"`
}

type Checkpoint struct {
	RunID     string
	Service   string
	Account   string
	Done      int
	Total     int
	Fetched   int
	CacheHits int
}

type CheckpointManifest struct {
	Format     int          `json:"format"`
	App        string       `json:"app"`
	Encrypted  bool         `json:"encrypted"`
	Incomplete bool         `json:"incomplete"`
	Exported   time.Time    `json:"exported"`
	Recipients []string     `json:"recipients,omitempty"`
	RunID      string       `json:"run_id"`
	Service    string       `json:"service"`
	Account    string       `json:"account,omitempty"`
	Done       int          `json:"done"`
	Total      int          `json:"total"`
	Fetched    int          `json:"fetched,omitempty"`
	CacheHits  int          `json:"cache_hits,omitempty"`
	Shards     []ShardEntry `json:"shards"`
}

type ShardEntry struct {
	Service string `json:"service"`
	Kind    string `json:"kind"`
	Account string `json:"account,omitempty"`
	Path    string `json:"path"`
	Rows    int    `json:"rows"`
	SHA256  string `json:"sha256"`
	Bytes   int64  `json:"bytes"`
}

type PlainShard struct {
	Service            string
	Kind               string
	Account            string
	Path               string
	Rows               int
	Plaintext          []byte
	PlaintextPath      string
	Existing           *ShardEntry
	ExistingRecipients []string
}

type Snapshot struct {
	Services []string
	Accounts []string
	Counts   map[string]int
	Shards   []PlainShard
}

type Result struct {
	Repo      string         `json:"repo"`
	Changed   bool           `json:"changed"`
	Encrypted bool           `json:"encrypted"`
	Shards    int            `json:"shards"`
	Counts    map[string]int `json:"counts,omitempty"`
}

func Init(ctx context.Context, opts Options) (Config, string, error) {
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return Config{}, "", err
	}
	recipient, err := EnsureIdentity(cfg.Identity)
	if err != nil {
		return Config{}, "", err
	}
	if len(cfg.Recipients) == 0 {
		cfg.Recipients = []string{recipient}
	}
	if err := SaveConfig(opts.ConfigPath, cfg); err != nil {
		return Config{}, "", err
	}
	if err := ensureRepo(ctx, cfg); err != nil {
		return Config{}, "", err
	}
	if err := writeBackupReadme(cfg.Repo); err != nil {
		return Config{}, "", err
	}
	_, err = commitAndPush(ctx, cfg, "docs: describe encrypted gog backup", opts.Push)
	return cfg, recipient, err
}

func PushSnapshot(ctx context.Context, snapshot Snapshot, opts Options) (Result, error) {
	defer cleanupPlainShardFiles(snapshot)
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return Result{}, err
	}
	if opts.Push {
		if err := waitAsyncPushes(ctx, cfg.Repo, opts.Progress); err != nil {
			return Result{}, err
		}
	}
	if len(cfg.Recipients) == 0 {
		recipient, err := RecipientFromIdentity(cfg.Identity)
		if err != nil {
			return Result{}, err
		}
		cfg.Recipients = []string{recipient}
	}
	if err := ensureRepo(ctx, cfg); err != nil {
		return Result{}, err
	}
	if err := writeBackupReadme(cfg.Repo); err != nil {
		return Result{}, err
	}
	oldManifest, _ := readManifest(cfg.Repo)
	manifest, err := writeSnapshot(ctx, cfg, snapshot, oldManifest)
	if err != nil {
		return Result{}, err
	}
	changed, err := commitAndPush(ctx, cfg, "sync: update encrypted gog backup", opts.Push)
	if err != nil {
		return Result{}, err
	}
	return Result{Repo: cfg.Repo, Changed: changed, Encrypted: true, Shards: len(manifest.Shards), Counts: manifest.Counts}, nil
}

func PushCheckpoint(ctx context.Context, snapshot Snapshot, checkpoint Checkpoint, opts Options) (Result, error) {
	defer cleanupPlainShardFiles(snapshot)
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return Result{}, err
	}
	if len(cfg.Recipients) == 0 {
		recipient, err := RecipientFromIdentity(cfg.Identity)
		if err != nil {
			return Result{}, err
		}
		cfg.Recipients = []string{recipient}
	}
	if opts.Push {
		if err := asyncPushError(cfg.Repo); err != nil {
			return Result{}, err
		}
	}
	if !opts.AsyncPush || !opts.Push || !asyncPusherActive(cfg.Repo) {
		if err := ensureRepo(ctx, cfg); err != nil {
			return Result{}, err
		}
	}
	if err := writeBackupReadme(cfg.Repo); err != nil {
		return Result{}, err
	}
	manifest, err := writeCheckpoint(ctx, cfg, snapshot, checkpoint)
	if err != nil {
		return Result{}, err
	}
	message := fmt.Sprintf("checkpoint: %s backup %d/%d", manifest.Service, manifest.Done, manifest.Total)
	changed, sha, err := commitChanges(ctx, cfg, message)
	if err != nil {
		return Result{}, err
	}
	if changed && opts.Push {
		if opts.AsyncPush {
			if err := enqueueAsyncPush(ctx, cfg, opts, sha, message); err != nil {
				return Result{}, err
			}
		} else if err := pushCommit(ctx, cfg, sha); err != nil {
			return Result{}, err
		}
	}
	counts := map[string]int{}
	for _, shard := range manifest.Shards {
		key := shard.Service
		if shard.Kind != "" {
			key += "." + shard.Kind
		}
		counts[key] += shard.Rows
	}
	return Result{Repo: cfg.Repo, Changed: changed, Encrypted: true, Shards: len(manifest.Shards), Counts: counts}, nil
}

func Verify(ctx context.Context, opts Options) (Result, error) {
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return Result{}, err
	}
	if err := ensureRepo(ctx, cfg); err != nil {
		return Result{}, err
	}
	manifest, err := readManifest(cfg.Repo)
	if err != nil {
		return Result{}, err
	}
	if manifest.Format != formatVersion {
		return Result{}, fmt.Errorf("unsupported backup format %d", manifest.Format)
	}
	counts := map[string]int{}
	for _, shard := range manifest.Shards {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}
		plaintext, err := decryptShardFile(cfg, shard)
		if err != nil {
			return Result{}, err
		}
		if got := sha256Hex(plaintext); got != shard.SHA256 {
			return Result{}, fmt.Errorf("backup shard hash mismatch for %s", shard.Path)
		}
		rows := countJSONLLines(plaintext)
		if rows != shard.Rows {
			return Result{}, fmt.Errorf("backup shard row count mismatch for %s: got %d, want %d", shard.Path, rows, shard.Rows)
		}
		key := shard.Service
		if strings.TrimSpace(shard.Kind) != "" {
			key += "." + shard.Kind
		}
		counts[key] += rows
	}
	return Result{Repo: cfg.Repo, Changed: false, Encrypted: manifest.Encrypted, Shards: len(manifest.Shards), Counts: counts}, nil
}

func Status(ctx context.Context, opts Options) (Manifest, string, error) {
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return Manifest{}, "", err
	}
	if err := ensureRepo(ctx, cfg); err != nil {
		return Manifest{}, "", err
	}
	manifest, err := readManifest(cfg.Repo)
	if err != nil {
		return Manifest{}, "", err
	}
	return manifest, cfg.Repo, nil
}

func NewJSONLShard(service, kind, account, rel string, rows any) (PlainShard, error) {
	plaintext, count, err := encodeJSONL(rows)
	if err != nil {
		return PlainShard{}, err
	}
	return PlainShard{
		Service:   strings.TrimSpace(service),
		Kind:      strings.TrimSpace(kind),
		Account:   strings.TrimSpace(account),
		Path:      filepath.ToSlash(rel),
		Rows:      count,
		Plaintext: plaintext,
	}, nil
}

func ExistingShard(entry ShardEntry, recipients []string) PlainShard {
	return PlainShard{
		Service:            strings.TrimSpace(entry.Service),
		Kind:               strings.TrimSpace(entry.Kind),
		Account:            strings.TrimSpace(entry.Account),
		Path:               filepath.ToSlash(entry.Path),
		Rows:               entry.Rows,
		Existing:           &entry,
		ExistingRecipients: append([]string(nil), recipients...),
	}
}

func ReadCheckpointManifest(repo, rel string) (CheckpointManifest, error) {
	return readCheckpointManifest(repo, rel)
}

func writeCheckpoint(ctx context.Context, cfg Config, snapshot Snapshot, checkpoint Checkpoint) (CheckpointManifest, error) {
	checkpoint.Service = safePathPart(checkpoint.Service)
	checkpoint.Account = safePathPart(checkpoint.Account)
	checkpoint.RunID = safePathPart(checkpoint.RunID)
	if checkpoint.Service == "" || checkpoint.RunID == "" {
		return CheckpointManifest{}, fmt.Errorf("backup checkpoint service and run id are required")
	}
	dir := path.Join("checkpoints", checkpoint.Service, checkpoint.Account, checkpoint.RunID)
	manifestRel := path.Join(dir, "manifest.json")
	old, _ := readCheckpointManifest(cfg.Repo, manifestRel)
	recipients := normalizedStrings(cfg.Recipients)
	reuseEncrypted := sameStrings(old.Recipients, recipients)
	replace := map[string]struct{}{}
	for _, shard := range snapshot.Shards {
		clean := path.Clean(strings.TrimSpace(shard.Path))
		if !strings.HasPrefix(clean, dir+"/") {
			return CheckpointManifest{}, fmt.Errorf("backup checkpoint shard path %q is outside %s", shard.Path, dir)
		}
		replace[clean] = struct{}{}
	}
	shards := make([]ShardEntry, 0, len(old.Shards)+len(snapshot.Shards))
	if reuseEncrypted {
		for _, shard := range old.Shards {
			if _, ok := replace[shard.Path]; !ok {
				shards = append(shards, shard)
			}
		}
	}
	oldManifest := Manifest{Recipients: old.Recipients, Shards: old.Shards}
	for _, shard := range snapshot.Shards {
		select {
		case <-ctx.Done():
			return CheckpointManifest{}, ctx.Err()
		default:
		}
		entry, err := writeShard(cfg, oldManifest, shard, reuseEncrypted)
		if err != nil {
			return CheckpointManifest{}, err
		}
		shards = append(shards, entry)
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].Path < shards[j].Path })
	manifest := CheckpointManifest{
		Format:     formatVersion,
		App:        "gog",
		Encrypted:  true,
		Incomplete: true,
		Exported:   time.Now().UTC(),
		Recipients: recipients,
		RunID:      checkpoint.RunID,
		Service:    checkpoint.Service,
		Account:    checkpoint.Account,
		Done:       checkpoint.Done,
		Total:      checkpoint.Total,
		Fetched:    checkpoint.Fetched,
		CacheHits:  checkpoint.CacheHits,
		Shards:     shards,
	}
	if err := writeCheckpointManifest(cfg.Repo, manifestRel, manifest); err != nil {
		return CheckpointManifest{}, err
	}
	return manifest, nil
}

func writeSnapshot(ctx context.Context, cfg Config, snapshot Snapshot, old Manifest) (Manifest, error) {
	recipients := normalizedStrings(cfg.Recipients)
	reuseEncrypted := sameStrings(old.Recipients, recipients)
	updatedServices := snapshotServices(snapshot)
	shards := make([]ShardEntry, 0, len(old.Shards)+len(snapshot.Shards))
	if reuseEncrypted {
		for _, shard := range old.Shards {
			if _, ok := updatedServices[shard.Service]; !ok {
				shards = append(shards, shard)
			}
		}
	}
	for _, shard := range snapshot.Shards {
		select {
		case <-ctx.Done():
			return Manifest{}, ctx.Err()
		default:
		}
		entry, err := writeShard(cfg, old, shard, reuseEncrypted)
		if err != nil {
			return Manifest{}, err
		}
		shards = append(shards, entry)
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].Path < shards[j].Path })
	manifest := Manifest{
		Format:     formatVersion,
		App:        "gog",
		Encrypted:  true,
		Exported:   time.Now().UTC(),
		Recipients: recipients,
		Services:   mergedManifestStrings(old.Services, snapshot.Services, reuseEncrypted),
		Accounts:   mergedManifestStrings(old.Accounts, snapshot.Accounts, reuseEncrypted),
		Counts:     mergedManifestCounts(old.Counts, snapshot.Counts, updatedServices, reuseEncrypted),
		Shards:     shards,
	}
	if manifest.Counts == nil {
		manifest.Counts = map[string]int{}
		for _, shard := range shards {
			key := shard.Service
			if shard.Kind != "" {
				key += "." + shard.Kind
			}
			manifest.Counts[key] += shard.Rows
		}
	}
	if equivalentManifest(old, manifest) {
		return old, nil
	}
	if err := removeStaleShards(cfg.Repo, shards); err != nil {
		return Manifest{}, err
	}
	if err := writeManifest(cfg.Repo, manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func snapshotServices(snapshot Snapshot) map[string]struct{} {
	services := map[string]struct{}{}
	for _, service := range snapshot.Services {
		service = strings.TrimSpace(service)
		if service != "" {
			services[service] = struct{}{}
		}
	}
	for _, shard := range snapshot.Shards {
		service := strings.TrimSpace(shard.Service)
		if service != "" {
			services[service] = struct{}{}
		}
	}
	return services
}

func mergedManifestStrings(old, next []string, preserveOld bool) []string {
	if !preserveOld {
		return normalizedStrings(next)
	}
	return normalizedStrings(append(append([]string(nil), old...), next...))
}

func mergedManifestCounts(old, next map[string]int, updatedServices map[string]struct{}, preserveOld bool) map[string]int {
	out := map[string]int{}
	if preserveOld {
		for key, value := range old {
			service, _, _ := strings.Cut(key, ".")
			if _, ok := updatedServices[service]; ok {
				continue
			}
			out[key] = value
		}
	}
	for key, value := range next {
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeShard(cfg Config, old Manifest, shard PlainShard, reuseEncrypted bool) (ShardEntry, error) {
	if strings.TrimSpace(shard.Service) == "" {
		return ShardEntry{}, fmt.Errorf("backup shard service is required")
	}
	if shard.Existing != nil {
		if len(shard.ExistingRecipients) > 0 && !sameStrings(shard.ExistingRecipients, cfg.Recipients) {
			return ShardEntry{}, fmt.Errorf("backup shard %s was encrypted for different recipients", shard.Existing.Path)
		}
		entry := *shard.Existing
		if strings.TrimSpace(entry.Service) == "" {
			entry.Service = shard.Service
		}
		if strings.TrimSpace(entry.Kind) == "" {
			entry.Kind = shard.Kind
		}
		if strings.TrimSpace(entry.Account) == "" {
			entry.Account = shard.Account
		}
		if strings.TrimSpace(entry.Path) == "" {
			entry.Path = shard.Path
		}
		if entry.Rows == 0 {
			entry.Rows = shard.Rows
		}
		path, err := resolveShardPath(cfg.Repo, entry.Path)
		if err != nil {
			return ShardEntry{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return ShardEntry{}, fmt.Errorf("reuse encrypted backup shard %s: %w", entry.Path, err)
		}
		if entry.Bytes > 0 && info.Size() != entry.Bytes {
			return ShardEntry{}, fmt.Errorf("reuse encrypted backup shard %s: size changed from %d to %d", entry.Path, entry.Bytes, info.Size())
		}
		entry.Bytes = info.Size()
		return entry, nil
	}
	hash, err := shardPlaintextHash(shard)
	if err != nil {
		return ShardEntry{}, err
	}
	path, err := resolveShardPath(cfg.Repo, shard.Path)
	if err != nil {
		return ShardEntry{}, err
	}
	if oldEntry, ok := old.entry(shard.Path); reuseEncrypted && ok && oldEntry.SHA256 == hash {
		if info, err := os.Stat(path); err == nil {
			oldEntry.Bytes = info.Size()
			return oldEntry, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return ShardEntry{}, err
	}
	bytesWritten, err := encryptShardToFile(shardPlaintextReader(shard), path, cfg.Recipients)
	if err != nil {
		return ShardEntry{}, err
	}
	return ShardEntry{
		Service: shard.Service,
		Kind:    shard.Kind,
		Account: shard.Account,
		Path:    shard.Path,
		Rows:    shard.Rows,
		SHA256:  hash,
		Bytes:   bytesWritten,
	}, nil
}

func cleanupPlainShardFiles(snapshot Snapshot) {
	for _, shard := range snapshot.Shards {
		if strings.TrimSpace(shard.PlaintextPath) != "" {
			_ = os.Remove(shard.PlaintextPath)
		}
	}
}

func shardPlaintextHash(shard PlainShard) (string, error) {
	if strings.TrimSpace(shard.PlaintextPath) == "" {
		return sha256Hex(shard.Plaintext), nil
	}
	f, err := os.Open(shard.PlaintextPath) // #nosec G304 -- PlaintextPath is created by gog as a temporary backup shard file.
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func shardPlaintextReader(shard PlainShard) func() (io.ReadCloser, error) {
	if strings.TrimSpace(shard.PlaintextPath) == "" {
		return func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(shard.Plaintext)), nil
		}
	}
	return func() (io.ReadCloser, error) {
		return os.Open(shard.PlaintextPath) // #nosec G304 -- PlaintextPath is created by gog as a temporary backup shard file.
	}
}

func decryptShardFile(cfg Config, shard ShardEntry) ([]byte, error) {
	path, err := resolveShardPath(cfg.Repo, shard.Path)
	if err != nil {
		return nil, err
	}
	ciphertext, err := os.ReadFile(path) // #nosec G304 -- resolveShardPath confines manifest-controlled shard paths to data/*.age inside the backup repo.
	if err != nil {
		return nil, err
	}
	return decryptShard(ciphertext, cfg.Identity)
}

func resolveShardPath(repo, rel string) (string, error) {
	clean := path.Clean(strings.TrimSpace(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("backup shard path escapes backup root: %s", rel)
	}
	if (!strings.HasPrefix(clean, "data/") && !strings.HasPrefix(clean, "checkpoints/")) || !strings.HasSuffix(clean, ".age") {
		return "", fmt.Errorf("invalid backup shard path: %s", rel)
	}
	full := filepath.Join(repo, filepath.FromSlash(clean))
	rootName := "data"
	if strings.HasPrefix(clean, "checkpoints/") {
		rootName = "checkpoints"
	}
	root := filepath.Clean(filepath.Join(repo, rootName))
	parent := filepath.Clean(filepath.Dir(full))
	if parent != root && !strings.HasPrefix(parent, root+string(filepath.Separator)) {
		return "", fmt.Errorf("backup shard path escapes backup root: %s", rel)
	}
	return full, nil
}

func encodeJSONL(rows any) ([]byte, int, error) {
	value := reflect.ValueOf(rows)
	if value.Kind() != reflect.Slice {
		return nil, 0, fmt.Errorf("unsupported JSONL rows %T", rows)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := 0; i < value.Len(); i++ {
		if err := enc.Encode(value.Index(i).Interface()); err != nil {
			return nil, 0, err
		}
	}
	return buf.Bytes(), value.Len(), nil
}

func DecodeJSONL[T any](plaintext []byte, out *[]T) error {
	for _, line := range jsonlLines(plaintext) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var value T
		if err := json.Unmarshal(line, &value); err != nil {
			return err
		}
		*out = append(*out, value)
	}
	return nil
}

func countJSONLLines(plaintext []byte) int {
	count := 0
	for _, line := range jsonlLines(plaintext) {
		if len(bytes.TrimSpace(line)) > 0 {
			count++
		}
	}
	return count
}

func jsonlLines(plaintext []byte) [][]byte {
	return bytes.Split(plaintext, []byte{'\n'})
}

func readManifest(repo string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(repo, "manifest.json")) // #nosec G304 -- repo is the configured local backup repository.
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func readCheckpointManifest(repo, rel string) (CheckpointManifest, error) {
	full, err := resolveCheckpointManifestPath(repo, rel)
	if err != nil {
		return CheckpointManifest{}, err
	}
	data, err := os.ReadFile(full) // #nosec G304 -- checkpoint manifest path is confined below checkpoints/.
	if err != nil {
		return CheckpointManifest{}, err
	}
	var manifest CheckpointManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return CheckpointManifest{}, err
	}
	return manifest, nil
}

func writeCheckpointManifest(repo, rel string, manifest CheckpointManifest) error {
	full, err := resolveCheckpointManifestPath(repo, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(full, data, 0o600)
}

func resolveCheckpointManifestPath(repo, rel string) (string, error) {
	clean := path.Clean(strings.TrimSpace(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("backup checkpoint path escapes backup root: %s", rel)
	}
	if !strings.HasPrefix(clean, "checkpoints/") || !strings.HasSuffix(clean, "/manifest.json") {
		return "", fmt.Errorf("invalid backup checkpoint path: %s", rel)
	}
	full := filepath.Join(repo, filepath.FromSlash(clean))
	root := filepath.Clean(filepath.Join(repo, "checkpoints"))
	parent := filepath.Clean(filepath.Dir(full))
	if parent != root && !strings.HasPrefix(parent, root+string(filepath.Separator)) {
		return "", fmt.Errorf("backup checkpoint path escapes backup root: %s", rel)
	}
	return full, nil
}

func writeManifest(repo string, manifest Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(repo, "manifest.json"), data, 0o600)
}

func (m Manifest) entry(path string) (ShardEntry, bool) {
	for _, shard := range m.Shards {
		if shard.Path == path {
			return shard, true
		}
	}
	return ShardEntry{}, false
}

func equivalentManifest(a, b Manifest) bool {
	if a.Format != b.Format ||
		a.App != b.App ||
		a.Encrypted != b.Encrypted ||
		!sameStrings(a.Recipients, b.Recipients) ||
		!sameStrings(a.Services, b.Services) ||
		!sameStrings(a.Accounts, b.Accounts) ||
		!reflect.DeepEqual(a.Counts, b.Counts) ||
		len(a.Shards) != len(b.Shards) {
		return false
	}
	for i := range a.Shards {
		left, right := a.Shards[i], b.Shards[i]
		left.Bytes, right.Bytes = 0, 0
		if left != right {
			return false
		}
	}
	return true
}

func normalizedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	a, b = normalizedStrings(a), normalizedStrings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func safePathPart(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func removeStaleShards(repo string, shards []ShardEntry) error {
	keep := map[string]struct{}{}
	for _, shard := range shards {
		keep[filepath.Clean(filepath.Join(repo, filepath.FromSlash(shard.Path)))] = struct{}{}
	}
	root := filepath.Join(repo, "data")
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	var stale []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".age") {
			return nil
		}
		clean := filepath.Clean(path)
		if _, ok := keep[clean]; ok {
			return nil
		}
		stale = append(stale, clean)
		return nil
	}); err != nil {
		return err
	}
	for _, path := range stale {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("stale shard path escapes backup root: %s", path)
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

func removeTempShardFiles(repo string) error {
	for _, rootName := range []string{"data", "checkpoints"} {
		root := filepath.Join(repo, rootName)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}
		if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return err
			}
			name := d.Name()
			if strings.HasPrefix(name, ".shard-") && strings.HasSuffix(name, ".age") {
				return os.Remove(path) //nolint:gosec // repo-owned temp shard paths come from WalkDir below configured backup roots.
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}
