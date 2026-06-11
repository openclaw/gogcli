package filelock

import (
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestSharedExclusiveSerializesCallers(t *testing.T) {
	lock := Shared(filepath.Join(t.TempDir(), "test.lock"), time.Second)
	var active atomic.Int32
	var maxActive atomic.Int32

	run := func(done chan<- error) {
		done <- lock.WithExclusive(func() error {
			current := active.Add(1)
			defer active.Add(-1)

			for {
				observedMax := maxActive.Load()
				if current <= observedMax || maxActive.CompareAndSwap(observedMax, current) {
					break
				}
			}

			time.Sleep(20 * time.Millisecond)

			return nil
		})
	}

	done := make(chan error, 2)
	go run(done)
	go run(done)

	for range 2 {
		if err := <-done; err != nil {
			t.Fatalf("lock: %v", err)
		}
	}

	if maxActive.Load() != 1 {
		t.Fatalf("max active = %d, want 1", maxActive.Load())
	}
}
