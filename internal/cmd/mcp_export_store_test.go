package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMCPExportStoreSingleFetch(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 1024, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var count atomic.Int32
	start := make(chan struct{})
	var fetch mcpSnapshotFetch = func(_ context.Context) ([]byte, gmailExportMetadata, error) {
		count.Add(1)
		<-start
		return []byte{0, 255, 13, 10}, gmailExportMetadata{MessageID: "m", ThreadID: "t"}, nil
	}
	key := mcpSnapshotKey{Partition: "account/client", Object: "raw:m"}
	var wg sync.WaitGroup
	ids := make(chan string, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lease, e := store.Acquire(context.Background(), key, 128, fetch)
			if e != nil {
				t.Error(e)
				return
			}
			defer lease.Release()
			ids <- lease.Info.ID
		}()
	}
	close(start)
	wg.Wait()
	close(ids)
	var id string
	for got := range ids {
		if id == "" {
			id = got
		}
		if got != id {
			t.Fatal("different snapshot IDs")
		}
	}
	if count.Load() != 1 {
		t.Fatalf("fetches %d", count.Load())
	}
	lease, err := store.AcquireID(key, id)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	data := make([]byte, 4)
	if _, err = lease.ReadAt(data, 0); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if lease.Info.SHA256 != hex.EncodeToString(sum[:]) || lease.Info.Size != 4 || lease.Info.Metadata.ThreadID != "t" {
		t.Fatal(lease.Info)
	}
	if _, err = store.AcquireID(mcpSnapshotKey{Partition: "other", Object: key.Object}, id); err == nil {
		t.Fatal("cross-account handle accepted")
	}
	info, err := os.Stat(store.dir)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatal(info.Mode())
	}
}

func TestMCPExportStoreAdmissionAndExpiry(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 16, Entries: 1, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	store.now = func() time.Time { return now }
	var fetch mcpSnapshotFetch = func(context.Context) ([]byte, gmailExportMetadata, error) {
		now = now.Add(time.Hour)
		return []byte("hello"), gmailExportMetadata{}, nil
	}
	key := mcpSnapshotKey{Partition: "p", Object: "a"}
	lease, err := store.Acquire(context.Background(), key, 8, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if !lease.Info.ExpiresAt.Equal(now.Add(900 * time.Second)) {
		t.Fatal("expiry starts before publication")
	}
	if _, err = store.Acquire(context.Background(), mcpSnapshotKey{Partition: "p", Object: "b"}, 8, fetch); !errors.Is(err, errMCPSnapshotCapacity) {
		t.Fatalf("unexpired handle evicted: %v", err)
	}
	now = now.Add(901 * time.Second)
	if _, err = store.AcquireID(key, lease.Info.ID); !errors.Is(err, errMCPSnapshotExpired) {
		t.Fatalf("expired handle: %v", err)
	}
	data := make([]byte, 5)
	if _, err = lease.ReadAt(data, 0); err != nil {
		t.Fatal("active lease lost", err)
	}
	lease.Release()
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(store.dir); !os.IsNotExist(err) {
		t.Fatal("snapshot files retained")
	}
}

func TestMCPExportStoreWaiterCancellation(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 128, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	cancelled := make(chan struct{})
	var fetch mcpSnapshotFetch = func(ctx context.Context) ([]byte, gmailExportMetadata, error) {
		close(started)
		select {
		case <-ctx.Done():
			close(cancelled)
			return nil, gmailExportMetadata{}, ctx.Err()
		case <-time.After(5 * time.Second):
			return []byte("unexpected completion"), gmailExportMetadata{}, nil
		}
	}
	done := make(chan error, 1)
	go func() { _, e := store.Acquire(ctx, mcpSnapshotKey{Object: "a"}, 64, fetch); done <- e }()
	<-started
	cancel()
	if err = <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("last waiter did not cancel fetch")
	}
}

func TestMCPExportStoreCloseWaitsForLease(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 128, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := store.Acquire(context.Background(), mcpSnapshotKey{Object: "a"}, 64, func(context.Context) ([]byte, gmailExportMetadata, error) {
		return []byte("data"), gmailExportMetadata{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- store.Close() }()
	select {
	case closeErr := <-done:
		t.Fatalf("close removed an active lease: %v", closeErr)
	case <-time.After(20 * time.Millisecond):
	}
	data := make([]byte, 4)
	if _, err = lease.ReadAt(data, 0); err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMCPExportStoreFirstWaiterCancellationKeepsSharedFetch(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 128, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var fetch mcpSnapshotFetch = func(ctx context.Context) ([]byte, gmailExportMetadata, error) {
		close(started)
		select {
		case <-ctx.Done():
			return nil, gmailExportMetadata{}, ctx.Err()
		case <-release:
			return []byte("intact"), gmailExportMetadata{}, nil
		}
	}
	key := mcpSnapshotKey{Object: "shared"}
	first := make(chan error, 1)
	second := make(chan error, 1)
	go func() {
		lease, e := store.Acquire(ctx, key, 64, fetch)
		if lease != nil {
			lease.Release()
		}
		first <- e
	}()
	<-started
	go func() {
		lease, e := store.Acquire(context.Background(), key, 64, fetch)
		if lease != nil {
			lease.Release()
		}
		second <- e
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		store.mu.Lock()
		waiters := store.pending[key].waiters
		store.mu.Unlock()
		if waiters == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second waiter failed to join")
		}
		runtime.Gosched()
	}
	cancel()
	if e := <-first; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	close(release)
	if e := <-second; e != nil {
		t.Fatal("first waiter cancelled another caller's fetch", e)
	}
}

func TestMCPExportStoreFetchAndReservationLimits(t *testing.T) {
	store, err := newMCPSnapshotStore(t.TempDir(), mcpSnapshotLimits{Bytes: 128, Entries: 8, Fetches: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 2)
	var fetch mcpSnapshotFetch = func(ctx context.Context) ([]byte, gmailExportMetadata, error) {
		started <- struct{}{}
		<-ctx.Done()
		return nil, gmailExportMetadata{}, ctx.Err()
	}
	done := make(chan error, 2)
	for _, object := range []string{"a", "b"} {
		go func() { _, e := store.Acquire(ctx, mcpSnapshotKey{Object: object}, 64, fetch); done <- e }()
	}
	for range 2 {
		<-started
	}
	if _, e := store.Acquire(context.Background(), mcpSnapshotKey{Object: "c"}, 1, fetch); !errors.Is(e, errMCPSnapshotCapacity) {
		t.Fatal("third fetch exceeded reservations/concurrency", e)
	}
	cancel()
	for range 2 {
		if e := <-done; !errors.Is(e, context.Canceled) {
			t.Fatal(e)
		}
	}
}
