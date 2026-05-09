package secrets

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"
)

func TestKeyringOperationTimeoutGuards(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		backend  string
		dbusAddr string
		wantWrap bool
	}{
		{name: "darwin auto", goos: "darwin", backend: "auto", wantWrap: true},
		{name: "darwin keychain", goos: "darwin", backend: "keychain", wantWrap: true},
		{name: "darwin file", goos: "darwin", backend: "file", wantWrap: false},
		{name: "linux auto with dbus", goos: "linux", backend: "auto", dbusAddr: "unix:path=/run/user/1000/bus", wantWrap: true},
		{name: "linux auto without dbus", goos: "linux", backend: "auto", wantWrap: false},
		{name: "linux keychain", goos: "linux", backend: "keychain", dbusAddr: "unix:path=/run/user/1000/bus", wantWrap: false},
		{name: "windows auto", goos: "windows", backend: "auto", wantWrap: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldUseKeyringOperationTimeout(tt.goos, KeyringBackendInfo{Value: tt.backend}, tt.dbusAddr)
			if got != tt.wantWrap {
				t.Fatalf("shouldUseKeyringOperationTimeout=%v, want %v", got, tt.wantWrap)
			}
		})
	}
}

func TestKeyringTimeoutHint(t *testing.T) {
	tests := []struct {
		goos       string
		wantSubstr string
	}{
		{"darwin", "Always Allow"},
		{"linux", "D-Bus SecretService"},
		{"windows", "keyring backend"},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			hint := keyringTimeoutHint(tt.goos)
			if !strings.Contains(hint, tt.wantSubstr) {
				t.Fatalf("keyringTimeoutHint(%q)=%q, want substring %q", tt.goos, hint, tt.wantSubstr)
			}
		})
	}
}

func TestTimeoutKeyringTimesOutOperations(t *testing.T) {
	block := make(chan struct{})

	t.Cleanup(func() { close(block) })

	ring := newTimeoutKeyring(&blockingKeyring{block: block}, 10*time.Millisecond, keyringTimeoutHint("darwin"))

	_, err := ring.Keys()
	if !errors.Is(err, errKeyringTimeout) {
		t.Fatalf("expected timeout error, got %v", err)
	}

	if !strings.Contains(err.Error(), "listing keyring items") || !strings.Contains(err.Error(), "Always Allow") {
		t.Fatalf("expected operation and macOS hint in timeout, got %v", err)
	}
}

type blockingKeyring struct {
	block <-chan struct{}
}

func (k *blockingKeyring) wait() {
	<-k.block
}

func (k *blockingKeyring) Get(string) (keyring.Item, error) {
	k.wait()
	return keyring.Item{}, keyring.ErrKeyNotFound
}

func (k *blockingKeyring) GetMetadata(string) (keyring.Metadata, error) {
	k.wait()
	return keyring.Metadata{}, keyring.ErrKeyNotFound
}

func (k *blockingKeyring) Set(keyring.Item) error {
	k.wait()
	return nil
}

func (k *blockingKeyring) Remove(string) error {
	k.wait()
	return nil
}

func (k *blockingKeyring) Keys() ([]string, error) {
	k.wait()
	return nil, nil
}
