package secrets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/99designs/keyring"
)

func TestRekey_TakesLiveLock(t *testing.T) {
	opts, dir := rekeyFixture(t)
	t.Setenv(keyringLockTimeoutEnv, "30ms")

	holder, err := os.OpenFile(filepath.Join(dir, keyringLockFilename), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	defer holder.Close()

	if err := lockKeyringFile(holder, true); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unlockKeyringFile(holder); err != nil {
			t.Error(err)
		}
	}()

	if _, err := Rekey(opts); !errors.Is(err, errKeyringTimeout) {
		t.Fatalf("expected lock timeout, got %v", err)
	}
}

func TestRekey_HoldsLiveLockThroughVerification(t *testing.T) {
	opts, dir := rekeyFixture(t)
	opens := 0

	opts.openOutput = func(path, password string) (keyring.Keyring, error) {
		opens++
		if opens == 2 {
			lock := &keyringLock{path: filepath.Join(dir, keyringLockFilename), timeout: 30 * time.Millisecond, mu: &sync.RWMutex{}}
			if err := lock.withWriteLock(func() error { return nil }); !errors.Is(err, errKeyringTimeout) {
				t.Fatalf("writer was not blocked: %v", err)
			}
		}

		return openRekeyOutput(path, password)
	}
	if _, err := Rekey(opts); err != nil {
		t.Fatal(err)
	}
}

func TestRekey_FailedWritePreservesOutput(t *testing.T) {
	opts, _ := rekeyFixture(t)

	opts.OutDir, opts.Force = t.TempDir(), true
	if err := os.WriteFile(filepath.Join(opts.OutDir, "keep"), []byte("previous output"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshotRekeyDir(t, opts.OutDir)

	opts.openOutput = func(string, string) (keyring.Keyring, error) { return nil, errRekeyTestWrite }
	if _, err := Rekey(opts); !errors.Is(err, errRekeyTestWrite) {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, opts.OutDir)) {
		t.Fatal("failed rekey destroyed previous output")
	}
}

func TestRekey_PublishRollback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory exchange is unavailable")
	}

	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			parent := t.TempDir()

			stage, out := filepath.Join(parent, "stage"), filepath.Join(parent, "output")
			if err := os.Mkdir(stage, 0o700); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(stage, "new"), []byte("new ciphertext"), 0o600); err != nil {
				t.Fatal(err)
			}

			if existing {
				if err := os.Mkdir(out, 0o700); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(filepath.Join(out, "old"), []byte("old ciphertext"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0

			err := publishRekeyOutputWith(stage, out, exchangeRekeyDirs, func(string) error {
				calls++
				if calls == 1 {
					return errRekeyTestWrite
				}

				return nil
			})
			if !errors.Is(err, errRekeyTestWrite) || calls != 2 {
				t.Fatalf("rollback not synced: %v (%d syncs)", err, calls)
			}

			if existing {
				data, err := os.ReadFile(filepath.Join(out, "old"))
				if err != nil || string(data) != "old ciphertext" {
					t.Fatal("previous output not restored")
				}

				if _, err := os.Stat(filepath.Join(out, "new")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("new output survived rollback")
				}
			} else if _, err := os.Stat(out); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed output was published")
			}
		})
	}
}

func TestRekey_ExistingOutputUnsupported(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only safety refusal")
	}
	opts, _ := rekeyFixture(t)

	opts.OutDir, opts.Force = t.TempDir(), true
	if err := os.WriteFile(filepath.Join(opts.OutDir, "keep"), []byte("old ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}

	before := snapshotRekeyDir(t, opts.OutDir)
	if _, err := Rekey(opts); err == nil || !strings.Contains(err.Error(), "fresh output directory") {
		t.Fatalf("expected safe refusal: %v", err)
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, opts.OutDir)) {
		t.Fatal("unsupported exchange modified output")
	}
}

func TestRekey_PublishesOnlyAfterVerification(t *testing.T) {
	opts, _ := rekeyFixture(t)
	opens := 0

	opts.openOutput = func(path, password string) (keyring.Keyring, error) {
		opens++

		if _, err := os.Stat(opts.OutDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("output visible before verification")
		}

		return openRekeyOutput(path, password)
	}
	if _, err := Rekey(opts); err != nil {
		t.Fatal(err)
	}

	if opens != 2 {
		t.Fatal("independent verification missing")
	}
}

func TestRekey_PublishRollbackFailureRetainsPreviousOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory exchange is unavailable")
	}
	parent := t.TempDir()

	stage, out := filepath.Join(parent, "stage"), filepath.Join(parent, "output")
	for _, path := range []string{stage, out} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(out, "old"), []byte("old ciphertext"), 0o600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := publishRekeyOutputWith(stage, out, func(a, b string) error {
		calls++
		if calls == 2 {
			return errRekeyTestWrite
		}

		return exchangeRekeyDirs(a, b)
	}, func(string) error { return errRekeyTestWrite })

	var recovery *rekeyRecoveryError
	if !errors.As(err, &recovery) || !strings.Contains(err.Error(), stage) {
		t.Fatalf("missing recovery path: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(stage, "old")); err != nil || string(data) != "old ciphertext" {
		t.Fatal("previous output lost after rollback failure")
	}
}
