package discoveryapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDescriptionCacheLifecycle(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = fmt.Fprint(w, `{"name":"demo","version":"v1"}`)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, CacheDir: t.TempDir()}
	load := func() {
		t.Helper()

		d, err := client.Description(context.Background(), "demo", "v1")
		if err != nil || d.Name != "demo" {
			t.Fatalf("description=%v err=%v", d, err)
		}
	}
	load()
	load()

	if requests.Load() != 1 {
		t.Fatalf("warm cache fetched %d times", requests.Load())
	}
	file := filepath.Join(client.CacheDir, client.descriptionCacheKey(server.URL+"/apis/demo/v1/rest"))

	for _, mode := range []string{"expired", "future", "corrupt"} {
		t.Run(mode, func(t *testing.T) {
			switch mode {
			case "expired":
				old := time.Now().Add(-descriptionCacheTTL - time.Hour)
				if err := os.Chtimes(file, old, old); err != nil {
					t.Fatal(err)
				}
			case "future":
				future := time.Now().Add(time.Hour)
				if err := os.Chtimes(file, future, future); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(file, []byte("not JSON"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			before := requests.Load()

			load()
			load()

			if requests.Load() != before+1 {
				t.Fatal("expected one refresh")
			}
		})
	}
	uncached := client
	uncached.CacheDir = ""
	before := requests.Load()

	for range 2 {
		if _, err := uncached.Description(context.Background(), "demo", "v1"); err != nil {
			t.Fatal(err)
		}
	}

	if requests.Load() != before+2 {
		t.Fatal("uncached path did not fetch each request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Description(ctx, "demo", "v1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cache lookup=%v", err)
	}
}

func TestDescriptionCacheDoesNotStoreFailures(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{500, `{"error":"fail"}`}, {200, "invalid"}} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			var count atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			client := Client{BaseURL: server.URL, CacheDir: t.TempDir()}
			for range 2 {
				if _, err := client.Description(context.Background(), "demo", "v1"); err == nil {
					t.Fatal("expected failure")
				}
			}

			if count.Load() != 2 {
				t.Fatal("cached failure")
			}

			entries, _ := os.ReadDir(client.CacheDir)
			if len(entries) != 0 {
				t.Fatalf("cached failure: %v", entries)
			}
		})
	}
}

func TestDescriptionCacheUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprint(w, `{"name":"demo"}`) }))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	client := Client{BaseURL: server.URL, CacheDir: path}
	if d, err := client.Description(context.Background(), "demo", "v1"); err != nil || d.Name != "demo" {
		t.Fatalf("cache error blocked network: %v", err)
	}
}

func TestDescriptionCacheKeyIsolation(t *testing.T) {
	a := Client{}
	b := Client{BaseURL: DefaultBaseURL}
	c := Client{BaseURL: "https://example.test"}

	url := DefaultBaseURL + "/apis/meet/v2/rest"
	if a.descriptionCacheKey(url) == b.descriptionCacheKey(url) {
		t.Fatal("fallback policy not isolated")
	}

	if b.descriptionCacheKey(url) == c.descriptionCacheKey(c.BaseURL+"/apis/meet/v2/rest") {
		t.Fatal("endpoints not isolated")
	}

	if b.descriptionCacheKey(url) == b.descriptionCacheKey(DefaultBaseURL+"/apis/meet/v1/rest") {
		t.Fatal("versions not isolated")
	}
}

func TestDescriptionCacheBoundsAndConcurrentWrites(t *testing.T) {
	client := Client{CacheDir: t.TempDir()}

	unrelated := filepath.Join(client.CacheDir, "keep.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	staleTemp := filepath.Join(client.CacheDir, descriptionCacheTempPrefix+"abandoned")
	if err := os.WriteFile(staleTemp, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := range descriptionCacheMaxEntries + 8 {
		wg.Go(func() { client.cacheDescription(client.descriptionCacheKey(fmt.Sprint(i)), []byte(`{"name":"demo"}`)) })
	}

	wg.Wait()

	entries, readDirErr := os.ReadDir(client.CacheDir)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	count := 0

	for _, e := range entries {
		if isDescriptionCacheName(e.Name()) {
			count++

			b, err := os.ReadFile(filepath.Join(client.CacheDir, e.Name()))
			if err != nil || !json.Valid(b) {
				t.Fatalf("partial record: %v", err)
			}
		}
	}

	if count > descriptionCacheMaxEntries {
		t.Fatalf("cached %d entries", count)
	}

	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("removed unrelated file")
	}

	if _, err := os.Stat(staleTemp); !os.IsNotExist(err) {
		t.Fatal("abandoned temp not removed")
	}
	// Sparse files prove byte eviction without allocating a large test buffer.
	for i := range 5 {
		path := filepath.Join(client.CacheDir, client.descriptionCacheKey(fmt.Sprint("large", i)))

		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = f.Truncate(descriptionCacheMaxEntryBytes)
		_ = f.Close()

		if err != nil {
			t.Fatal(err)
		}
	}

	client.cacheDescription(client.descriptionCacheKey("last"), []byte(`{"name":"last"}`))

	entries, readDirErr = os.ReadDir(client.CacheDir)
	if readDirErr != nil {
		t.Fatal(readDirErr)
	}
	var total int64

	for _, e := range entries {
		if isDescriptionCacheName(e.Name()) {
			info, err := e.Info()
			if err != nil {
				t.Fatal(err)
			}
			total += info.Size()
		}
	}

	if total > descriptionCacheMaxBytes {
		t.Fatalf("cached %d bytes", total)
	}
}

func TestDescriptionCacheFallbackAndDirectoryRemainIndependent(t *testing.T) {
	var calls atomic.Int32

	client := Client{CacheDir: t.TempDir(), HTTP: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		code := 200
		body := `{"name":"meet","version":"v2"}`

		if strings.Contains(r.URL.Path, "/apis/meet/") {
			code = 404
			body = `{}`
		}

		return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}}
	for range 2 {
		if _, err := client.Description(context.Background(), "meet", "v2"); err != nil {
			t.Fatal(err)
		}
	}

	if calls.Load() != 2 {
		t.Fatalf("fallback cache requests=%d", calls.Load())
	}

	for range 2 {
		if _, err := client.List(context.Background(), true); err != nil {
			t.Fatal(err)
		}
	}

	if calls.Load() != 4 {
		t.Fatal("directory listing was cached")
	}
	explicit := client

	explicit.BaseURL = DefaultBaseURL
	if _, err := explicit.Description(context.Background(), "meet", "v2"); err == nil {
		t.Fatal("explicit endpoint reused fallback entry")
	}
}
