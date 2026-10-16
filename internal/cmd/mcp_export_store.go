package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	errMCPSnapshotCapacity = errors.New("snapshot capacity exhausted")
	errMCPSnapshotExpired  = errors.New("snapshot unavailable or expired")
	errMCPSnapshotMismatch = errors.New("snapshot object mismatch")
	errMCPSnapshotClosed   = errors.New("snapshot store closed")
)

type (
	mcpSnapshotKey    struct{ Partition, Object string }
	mcpSnapshotLimits struct {
		Bytes            int64
		Entries, Fetches int
	}
)

type mcpSnapshotInfo struct {
	ID        string              `json:"snapshot_id"`
	Size      int64               `json:"size"`
	SHA256    string              `json:"sha256"`
	ExpiresAt time.Time           `json:"expires_at"`
	Metadata  gmailExportMetadata `json:"-"`
}
type (
	mcpSnapshotFetch func(context.Context) ([]byte, gmailExportMetadata, error)
	mcpSnapshotEntry struct {
		key     mcpSnapshotKey
		info    mcpSnapshotInfo
		file    *os.File
		refs    int
		expired bool
	}
)

type mcpSnapshotPending struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	abandoned bool
	reserve   int64
	entry     *mcpSnapshotEntry
	err       error
}
type mcpSnapshotStore struct {
	mu      sync.Mutex
	dir     string
	limits  mcpSnapshotLimits
	now     func() time.Time
	entries map[string]*mcpSnapshotEntry
	keys    map[mcpSnapshotKey]*mcpSnapshotEntry
	pending map[mcpSnapshotKey]*mcpSnapshotPending
	bytes   int64
	wg      sync.WaitGroup
	leases  sync.WaitGroup
	closed  bool
}
type mcpSnapshotLease struct {
	Info  mcpSnapshotInfo
	store *mcpSnapshotStore
	entry *mcpSnapshotEntry
	once  sync.Once
}

func newMCPSnapshotStore(parent string, limits mcpSnapshotLimits) (*mcpSnapshotStore, error) {
	if limits.Bytes <= 0 || limits.Entries <= 0 || limits.Fetches <= 0 {
		return nil, fmt.Errorf("invalid snapshot limits")
	}
	dir, err := os.MkdirTemp(parent, "gog-mcp-")
	if err != nil {
		return nil, err
	}
	if err = makeMCPStoragePrivate(dir, true); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &mcpSnapshotStore{dir: dir, limits: limits, now: time.Now, entries: map[string]*mcpSnapshotEntry{}, keys: map[mcpSnapshotKey]*mcpSnapshotEntry{}, pending: map[mcpSnapshotKey]*mcpSnapshotPending{}}, nil
}

func (s *mcpSnapshotStore) Acquire(ctx context.Context, key mcpSnapshotKey, reserve int64, fetch mcpSnapshotFetch) (*mcpSnapshotLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errMCPSnapshotClosed
	}
	s.expireLocked()
	if entry := s.keys[key]; entry != nil {
		lease := s.leaseLocked(entry)
		s.mu.Unlock()
		return lease, nil
	}
	p := s.pending[key]
	if p == nil {
		if reserve <= 0 || reserve > s.limits.Bytes-s.bytes || len(s.entries)+len(s.pending) >= s.limits.Entries || len(s.pending) >= s.limits.Fetches {
			s.mu.Unlock()
			return nil, errMCPSnapshotCapacity
		}
		// A fetch is owned by all its waiters, rather than the first request.
		fetchCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		p = &mcpSnapshotPending{done: make(chan struct{}), cancel: cancel, reserve: reserve}
		s.pending[key] = p
		s.bytes += reserve
		s.wg.Add(1)
		go s.fetch(fetchCtx, key, p, fetch)
	}
	p.waiters++
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		s.mu.Lock()
		p.waiters--
		if p.waiters == 0 {
			p.abandoned = true
			p.cancel()
		}
		s.mu.Unlock()
		return nil, ctx.Err()
	case <-p.done:
		s.mu.Lock()
		p.waiters--
		if err := ctx.Err(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if p.abandoned && p.err != nil {
			// The previous last waiter cancelled this fetch before we joined.
			// Completion has released its reservation; start a fresh fetch.
			s.mu.Unlock()
			return s.Acquire(ctx, key, reserve, fetch)
		}
		if p.err != nil {
			s.mu.Unlock()
			return nil, p.err
		}
		if s.closed || p.entry.expired {
			s.mu.Unlock()
			return nil, errMCPSnapshotExpired
		}
		lease := s.leaseLocked(p.entry)
		s.mu.Unlock()
		return lease, nil
	}
}

func (s *mcpSnapshotStore) fetch(ctx context.Context, key mcpSnapshotKey, p *mcpSnapshotPending, fetch mcpSnapshotFetch) {
	defer s.wg.Done()
	defer p.cancel()
	data, info, err := fetch(ctx)
	var entry *mcpSnapshotEntry
	if err == nil && int64(len(data)) > p.reserve {
		err = errMCPSnapshotCapacity
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		var id [16]byte
		if _, err = rand.Read(id[:]); err == nil {
			name := hex.EncodeToString(id[:])
			path := filepath.Join(s.dir, name)
			var file *os.File
			//nolint:gosec // private generated directory and random basename
			file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if err == nil {
				err = makeMCPStoragePrivate(path, false)
				if err == nil {
					_, err = file.Write(data)
				}
				if err == nil {
					err = file.Sync()
				}
				if err == nil {
					sum := sha256.Sum256(data)
					info.Size = int64(len(data))
					entry = &mcpSnapshotEntry{key: key, file: file, info: mcpSnapshotInfo{ID: name, Size: info.Size, SHA256: hex.EncodeToString(sum[:]), Metadata: info}}
				} else {
					_ = file.Close()
					_ = os.Remove(path)
				}
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, key)
	s.bytes -= p.reserve
	if err == nil && (s.closed || ctx.Err() != nil) {
		err = errMCPSnapshotClosed
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	}
	if err != nil {
		if entry != nil {
			_ = entry.file.Close()
			_ = os.Remove(entry.file.Name())
		}
		p.err = err
	} else {
		entry.info.ExpiresAt = s.now().Add(900 * time.Second)
		s.entries[entry.info.ID] = entry
		s.keys[key] = entry
		s.bytes += entry.info.Size
		p.entry = entry
	}
	close(p.done)
}

func (s *mcpSnapshotStore) AcquireID(key mcpSnapshotKey, id string) (*mcpSnapshotLease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errMCPSnapshotClosed
	}
	s.expireLocked()
	entry := s.entries[id]
	if entry == nil || entry.expired {
		return nil, errMCPSnapshotExpired
	}
	if entry.key != key {
		return nil, errMCPSnapshotMismatch
	}
	return s.leaseLocked(entry), nil
}

func (s *mcpSnapshotStore) leaseLocked(entry *mcpSnapshotEntry) *mcpSnapshotLease {
	entry.refs++
	s.leases.Add(1)
	return &mcpSnapshotLease{Info: entry.info, store: s, entry: entry}
}

func (l *mcpSnapshotLease) ReadAt(data []byte, offset int64) (int, error) {
	return l.entry.file.ReadAt(data, offset)
}

func (l *mcpSnapshotLease) Release() {
	l.once.Do(func() {
		s := l.store
		s.mu.Lock()
		defer s.mu.Unlock()
		l.entry.refs--
		s.leases.Done()
		if l.entry.expired && l.entry.refs == 0 {
			s.removeLocked(l.entry)
		}
	})
}

func (s *mcpSnapshotStore) expireLocked() {
	now := s.now()
	for _, e := range s.entries {
		if !e.info.ExpiresAt.After(now) {
			e.expired = true
			if s.keys[e.key] == e {
				delete(s.keys, e.key)
			}
			if e.refs == 0 {
				s.removeLocked(e)
			}
		}
	}
}

func (s *mcpSnapshotStore) removeLocked(e *mcpSnapshotEntry) {
	delete(s.entries, e.info.ID)
	if s.keys[e.key] == e {
		delete(s.keys, e.key)
	}
	s.bytes -= e.info.Size
	_ = e.file.Close()
	_ = os.Remove(e.file.Name())
}

func (s *mcpSnapshotStore) Close() error {
	s.mu.Lock()
	s.closed = true
	for _, p := range s.pending {
		p.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.leases.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.entries {
		_ = e.file.Close()
	}
	s.entries = map[string]*mcpSnapshotEntry{}
	s.keys = map[mcpSnapshotKey]*mcpSnapshotEntry{}
	return os.RemoveAll(s.dir)
}
