package discoveryapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	discovery "google.golang.org/api/discovery/v1"

	"github.com/openclaw/gogcli/internal/filelock"
)

const (
	descriptionCacheTTL           = 24 * time.Hour
	descriptionCacheMaxEntries    = 32
	descriptionCacheMaxBytes      = 64 << 20
	descriptionCacheMaxEntryBytes = 16 << 20
	descriptionCacheTempPrefix    = ".discovery-"
)

func (c Client) descriptionCacheKey(requestURL string) string {
	// Explicit base URLs must not reuse service-hosted fallback results.
	if strings.TrimSpace(c.BaseURL) == "" {
		requestURL += "\x00service-hosted-fallback"
	}
	sum := sha256.Sum256([]byte(requestURL))

	return hex.EncodeToString(sum[:]) + ".json"
}

func (c Client) cachedDescription(key string) *discovery.RestDescription {
	path := filepath.Join(c.CacheDir, key)

	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > descriptionCacheMaxEntryBytes {
		return nil
	}

	age := time.Since(info.ModTime())
	if age < 0 || age >= descriptionCacheTTL {
		return nil
	}

	f, err := os.Open(path) // #nosec G304 -- key is a generated SHA-256 filename in the configured cache directory.
	if err != nil {
		return nil
	}
	defer f.Close()

	raw, err := io.ReadAll(io.LimitReader(f, descriptionCacheMaxEntryBytes+1))
	if err != nil || len(raw) > descriptionCacheMaxEntryBytes {
		return nil
	}

	var description discovery.RestDescription
	if json.Unmarshal(raw, &description) != nil {
		return nil
	}

	return &description
}

func (c Client) cacheDescription(key string, raw []byte) {
	if c.CacheDir == "" || len(raw) > descriptionCacheMaxEntryBytes {
		return
	}

	if os.MkdirAll(c.CacheDir, 0o700) != nil {
		return
	}
	// Serialize pruning and replacement across CLI processes; never hold a lock
	// over the network, and treat contention or filesystem errors as cache misses.
	_ = filelock.Shared(filepath.Join(c.CacheDir, ".lock"), 100*time.Millisecond).WithExclusive(func() error {
		if !pruneDescriptionCache(c.CacheDir, key, int64(len(raw))) {
			return nil
		}

		f, err := os.CreateTemp(c.CacheDir, descriptionCacheTempPrefix)
		if err != nil {
			return nil
		}
		defer os.Remove(f.Name())

		if _, err := f.Write(raw); err != nil {
			_ = f.Close()
			return nil
		}

		if err := f.Close(); err != nil {
			return nil
		}
		_ = os.Rename(f.Name(), filepath.Join(c.CacheDir, key))

		return nil
	})
}

func pruneDescriptionCache(dir, replacing string, incoming int64) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	type record struct {
		name string
		info os.FileInfo
	}
	var records []record
	total := incoming

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, descriptionCacheTempPrefix) && entry.Type().IsRegular() {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return false
			}

			continue
		}

		if name == replacing || !isDescriptionCacheName(name) || !entry.Type().IsRegular() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			return false
		}
		total += info.Size()
		records = append(records, record{name: name, info: info})
	}

	sort.Slice(records, func(i, j int) bool { return records[i].info.ModTime().Before(records[j].info.ModTime()) })

	count := len(records) + 1
	for _, record := range records {
		age := time.Since(record.info.ModTime())
		if age >= 0 && age < descriptionCacheTTL && count <= descriptionCacheMaxEntries && total <= descriptionCacheMaxBytes {
			continue
		}

		if err := os.Remove(filepath.Join(dir, record.name)); err != nil {
			return false
		}
		total -= record.info.Size()
		count--
	}

	return true
}

func isDescriptionCacheName(name string) bool {
	if len(name) != sha256.Size*2+len(".json") || !strings.HasSuffix(name, ".json") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimSuffix(name, ".json"))

	return err == nil
}
