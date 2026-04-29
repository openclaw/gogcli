//nolint:err113,wrapcheck,wsl_v5 // Contextual errors keep backup call sites readable.
package backup

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func Cat(ctx context.Context, opts Options, shardPath string) (PlainShard, error) {
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return PlainShard{}, err
	}
	if !opts.SkipPull {
		repoErr := ensureRepo(ctx, cfg)
		if repoErr != nil {
			return PlainShard{}, repoErr
		}
	} else if strings.TrimSpace(cfg.Repo) == "" {
		return PlainShard{}, fmt.Errorf("backup repo path is required")
	}
	manifest, err := readManifest(cfg.Repo)
	if err != nil {
		return PlainShard{}, err
	}
	if manifest.Format != formatVersion {
		return PlainShard{}, fmt.Errorf("unsupported backup format %d", manifest.Format)
	}
	shard, err := findManifestShard(manifest, cfg.Repo, shardPath)
	if err != nil {
		return PlainShard{}, err
	}
	return decryptManifestShard(cfg, shard)
}

func DecryptSnapshot(ctx context.Context, opts Options) (Manifest, string, []PlainShard, error) {
	shards := []PlainShard{}
	manifest, repo, err := WalkSnapshot(ctx, opts, func(_ Manifest, _ string, shard PlainShard) error {
		shards = append(shards, shard)
		return nil
	})
	return manifest, repo, shards, err
}

func WalkSnapshot(ctx context.Context, opts Options, visit func(Manifest, string, PlainShard) error) (Manifest, string, error) {
	cfg, err := ResolveOptions(opts)
	if err != nil {
		return Manifest{}, "", err
	}
	if !opts.SkipPull {
		repoErr := ensureRepo(ctx, cfg)
		if repoErr != nil {
			return Manifest{}, "", repoErr
		}
	} else if strings.TrimSpace(cfg.Repo) == "" {
		return Manifest{}, "", fmt.Errorf("backup repo path is required")
	}
	manifest, err := readManifest(cfg.Repo)
	if err != nil {
		return Manifest{}, "", err
	}
	if manifest.Format != formatVersion {
		return Manifest{}, "", fmt.Errorf("unsupported backup format %d", manifest.Format)
	}
	for _, shard := range manifest.Shards {
		select {
		case <-ctx.Done():
			return Manifest{}, "", ctx.Err()
		default:
		}
		plain, err := decryptManifestShard(cfg, shard)
		if err != nil {
			return Manifest{}, "", err
		}
		if visit != nil {
			if err := visit(manifest, cfg.Repo, plain); err != nil {
				return Manifest{}, "", err
			}
		}
	}
	return manifest, cfg.Repo, nil
}

func decryptManifestShard(cfg Config, shard ShardEntry) (PlainShard, error) {
	plaintext, err := decryptShardFile(cfg, shard)
	if err != nil {
		return PlainShard{}, err
	}
	if err := verifyPlainShard(shard, plaintext); err != nil {
		return PlainShard{}, err
	}
	return PlainShard{
		Service:   shard.Service,
		Kind:      shard.Kind,
		Account:   shard.Account,
		Path:      shard.Path,
		Rows:      shard.Rows,
		Plaintext: plaintext,
	}, nil
}

func verifyPlainShard(shard ShardEntry, plaintext []byte) error {
	if got := sha256Hex(plaintext); got != shard.SHA256 {
		return fmt.Errorf("backup shard hash mismatch for %s", shard.Path)
	}
	rows := countJSONLLines(plaintext)
	if rows != shard.Rows {
		return fmt.Errorf("backup shard row count mismatch for %s: got %d, want %d", shard.Path, rows, shard.Rows)
	}
	return nil
}

func findManifestShard(manifest Manifest, repo, shardPath string) (ShardEntry, error) {
	ref, err := normalizeShardRef(repo, shardPath)
	if err != nil {
		return ShardEntry{}, err
	}
	if shard, ok := manifest.entry(ref); ok {
		return shard, nil
	}
	return ShardEntry{}, fmt.Errorf("backup shard not found in manifest: %s", shardPath)
}

func normalizeShardRef(repo, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("backup shard path is required")
	}
	if filepath.IsAbs(ref) {
		rel, err := filepath.Rel(repo, ref)
		if err != nil {
			return "", err
		}
		ref = rel
	}
	clean := path.Clean(filepath.ToSlash(ref))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("backup shard path escapes backup root: %s", ref)
	}
	return clean, nil
}
