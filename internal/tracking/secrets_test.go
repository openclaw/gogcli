package tracking

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/99designs/keyring"
)

var (
	errTestSetSecret  = errors.New("set failed")
	errTestReadSecret = errors.New("read failed")
)

type memorySecretBackend struct {
	mu         sync.Mutex
	values     map[string][]byte
	failSetKey string
	getErrors  map[string]error
}

func newMemorySecretBackend() *memorySecretBackend {
	return &memorySecretBackend{
		values:    map[string][]byte{},
		getErrors: map[string]error{},
	}
}

func (s *memorySecretBackend) SetSecret(key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if key == s.failSetKey {
		return errTestSetSecret
	}

	s.values[key] = append([]byte(nil), value...)

	return nil
}

func (s *memorySecretBackend) GetSecret(key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.getErrors[key]; err != nil {
		return nil, err
	}

	value, ok := s.values[key]
	if !ok {
		return nil, keyring.ErrKeyNotFound
	}

	return append([]byte(nil), value...), nil
}

func (s *memorySecretBackend) DeleteSecret(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.values[key]; !ok {
		return keyring.ErrKeyNotFound
	}

	delete(s.values, key)

	return nil
}

func newTestSecretStore(t *testing.T) (*SecretStore, *memorySecretBackend) {
	t.Helper()

	backend := newMemorySecretBackend()

	store, err := NewSecretStore(backend)
	if err != nil {
		t.Fatalf("NewSecretStore: %v", err)
	}

	return store, backend
}

func TestNewSecretStoreRequiresBackend(t *testing.T) {
	if _, err := NewSecretStore(nil); !errors.Is(err, errNilSecretStore) {
		t.Fatalf("NewSecretStore error = %v", err)
	}
}

func TestSecretStoreSaveAndLoadSecrets(t *testing.T) {
	store, _ := newTestSecretStore(t)

	if err := store.SaveSecrets("a@b.com", "track", "admin"); err != nil {
		t.Fatalf("SaveSecrets: %v", err)
	}

	track, admin, err := store.LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "track" || admin != "admin" {
		t.Fatalf("unexpected secrets: %q %q", track, admin)
	}
}

func TestSecretStoreSaveAndLoadTrackingKeys(t *testing.T) {
	store, _ := newTestSecretStore(t)

	keys := map[int]string{
		1: "track-v1",
		2: "track-v2",
	}
	if err := store.SaveTrackingKeys("a@b.com", keys, 2, "admin"); err != nil {
		t.Fatalf("SaveTrackingKeys: %v", err)
	}

	loaded, currentVersion, err := store.LoadTrackingKeys("a@b.com", []int{1, 2}, 2)
	if err != nil {
		t.Fatalf("LoadTrackingKeys: %v", err)
	}

	if currentVersion != 2 {
		t.Fatalf("current version = %d, want 2", currentVersion)
	}

	if loaded[1] != "track-v1" || loaded[2] != "track-v2" {
		t.Fatalf("unexpected tracking keys: %#v", loaded)
	}

	track, admin, err := store.LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "track-v2" || admin != "admin" {
		t.Fatalf("unexpected current secrets: %q %q", track, admin)
	}
}

func TestSecretStoreLegacyFallback(t *testing.T) {
	store, backend := newTestSecretStore(t)

	if err := backend.SetSecret(legacyTrackingKeySecretKey, []byte("legacy-track")); err != nil {
		t.Fatalf("SetSecret legacy: %v", err)
	}

	if err := backend.SetSecret(legacyAdminKeySecretKey, []byte("legacy-admin")); err != nil {
		t.Fatalf("SetSecret legacy admin: %v", err)
	}

	track, admin, err := store.LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "legacy-track" || admin != "legacy-admin" {
		t.Fatalf("unexpected legacy secrets: %q %q", track, admin)
	}
}

func TestSecretStoreAccountIsolation(t *testing.T) {
	store, _ := newTestSecretStore(t)

	if err := store.SaveSecrets("a@b.com", "track-a", "admin-a"); err != nil {
		t.Fatalf("SaveSecrets a: %v", err)
	}

	if err := store.SaveSecrets("c@d.com", "track-c", "admin-c"); err != nil {
		t.Fatalf("SaveSecrets c: %v", err)
	}

	track, admin, err := store.LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "track-a" || admin != "admin-a" {
		t.Fatalf("unexpected account secrets: %q %q", track, admin)
	}
}

func TestSecretStoreMissingKeys(t *testing.T) {
	store, _ := newTestSecretStore(t)

	track, admin, err := store.LoadSecrets("a@b.com")
	if err != nil {
		t.Fatalf("LoadSecrets: %v", err)
	}

	if track != "" || admin != "" {
		t.Fatalf("unexpected missing secrets: %q %q", track, admin)
	}
}

func TestSecretStoreSaveFailureAfterVersionedKeys(t *testing.T) {
	store, backend := newTestSecretStore(t)
	account := "a@b.com"
	backend.failSetKey = scopedSecretKey(account, trackingKeySecretSuffix)

	err := store.SaveTrackingKeys(account, map[int]string{
		2: "track-v2",
		1: "track-v1",
	}, 2, "admin")
	if err == nil || !strings.Contains(err.Error(), "store tracking key: set secret: set failed") {
		t.Fatalf("SaveTrackingKeys error = %v", err)
	}

	for _, version := range []int{1, 2} {
		key := scopedSecretKey(account, versionedTrackingKeySecretSuffix(version))
		if string(backend.values[key]) == "" {
			t.Fatalf("versioned key %d not written before failure", version)
		}
	}

	if _, ok := backend.values[scopedSecretKey(account, adminKeySecretSuffix)]; ok {
		t.Fatalf("admin key written after current-key failure")
	}
}

func TestSecretStoreReadError(t *testing.T) {
	store, backend := newTestSecretStore(t)
	key := scopedSecretKey("a@b.com", trackingKeySecretSuffix)
	backend.getErrors[key] = errTestReadSecret

	if _, _, err := store.LoadSecrets("a@b.com"); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("LoadSecrets error = %v", err)
	}
}

func TestScopedSecretKey(t *testing.T) {
	if got := scopedSecretKey(" A@B.com ", "tracking_key"); got != "tracking/A@B.com/tracking_key" {
		t.Fatalf("unexpected scoped key: %q", got)
	}
}
