package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/99designs/keyring"
)

var (
	errIncompleteServiceAccountLayout = errors.New("incomplete service account layout")
	errServiceAccountSecretsMissing   = errors.New("service account key is stored in the keyring, but no keyring is available")
)

const serviceAccountKeyringKeyPrefix = "service-account/"

// ServiceAccountSecrets is the keyring a service account key can be stored in instead of the plain file.
type ServiceAccountSecrets interface {
	SetSecret(key string, value []byte) error
	GetSecret(key string) ([]byte, error)
	DeleteSecret(key string) error
}

// serviceAccountKeyringRef is what the service account file holds when the key itself is in the keyring.
type serviceAccountKeyringRef struct {
	KeyringKey string `json:"gog_keyring_key"`
}

type ServiceAccountFile struct {
	Path         string
	Data         []byte
	ModifiedAt   time.Time
	KeepSpecific bool
}

type ServiceAccountStore struct {
	layout      Layout
	openSecrets func() (ServiceAccountSecrets, error)
}

func NewServiceAccountStore(layout Layout) *ServiceAccountStore {
	return &ServiceAccountStore{layout: layout}
}

// WithSecrets sets the keyring used by WriteKeyring and by reads of a key stored there.
func (s *ServiceAccountStore) WithSecrets(open func() (ServiceAccountSecrets, error)) *ServiceAccountStore {
	s.openSecrets = open

	return s
}

func (s *ServiceAccountStore) Layout() Layout {
	if s == nil {
		return Layout{}
	}

	return s.layout
}

func (s *ServiceAccountStore) Path(email string) string {
	return s.layout.ServiceAccountPath(email)
}

func (s *ServiceAccountStore) KeepPath(email string) string {
	return s.layout.KeepServiceAccountPath(email)
}

func (s *ServiceAccountStore) Write(email string, data []byte) (string, error) {
	if err := s.validateLayout(false); err != nil {
		return "", err
	}

	if err := s.ensureDataDir(); err != nil {
		return "", err
	}

	path := s.Path(email)
	oldKey, hadKeyringCopy := s.keyringKeyAt(path)

	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write service account: %w", err)
	}

	// Back to file storage: the key the old reference named leaves the keyring.
	if hadKeyringCopy {
		if err := s.deleteKeyringKey(oldKey); err != nil {
			return "", err
		}
	}

	return path, nil
}

// WriteKeyring stores the key in the keyring. The service account file keeps only a reference to it,
// and a plain copy at the legacy path is removed.
func (s *ServiceAccountStore) WriteKeyring(email string, data []byte) (string, error) {
	if err := s.validateLayout(false); err != nil {
		return "", err
	}

	store, err := s.secrets()
	if err != nil {
		return "", err
	}

	if dirErr := s.ensureDataDir(); dirErr != nil {
		return "", dirErr
	}
	path := s.Path(email)
	oldKey, hadKeyringCopy := s.keyringKeyAt(path)
	// A fresh key keeps failed writes and separate layouts from replacing a live credential.
	key := serviceAccountKeyringKeyPrefix + strings.ToLower(strings.TrimSpace(email)) + "/" + rand.Text()
	if setErr := store.SetSecret(key, data); setErr != nil {
		return "", fmt.Errorf("store service account key in keyring: %w", setErr)
	}

	ref, err := json.Marshal(serviceAccountKeyringRef{KeyringKey: key})
	if err != nil {
		return "", fmt.Errorf("encode service account keyring reference: %w", err)
	}

	if err := WriteFileAtomic(path, ref, 0o600); err != nil {
		cleanupErr := store.DeleteSecret(key)
		return "", errors.Join(fmt.Errorf("write service account: %w", err), cleanupErr)
	}

	if hadKeyringCopy {
		if err := s.deleteKeyringKey(oldKey); err != nil {
			return "", err
		}
	}

	if !s.layout.ExplicitData {
		legacy := s.layout.ServiceAccountLegacyPath(email)
		if filepath.Clean(legacy) != filepath.Clean(path) {
			if err := os.Remove(legacy); err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("remove plain service account file: %w", err)
			}
		}
	}

	return path, nil
}

func (s *ServiceAccountStore) WriteKeep(email string, data []byte) (string, error) {
	if err := s.validateLayout(false); err != nil {
		return "", err
	}

	if err := s.ensureDataDir(); err != nil {
		return "", err
	}

	path := s.KeepPath(email)
	if err := WriteFileAtomic(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write Keep service account: %w", err)
	}

	return path, nil
}

func (s *ServiceAccountStore) WriteKeepCompatibility(email string, data []byte) ([]string, error) {
	keepPath, err := s.WriteKeep(email, data)
	if err != nil {
		return nil, err
	}

	genericPath, err := s.Write(email, data)
	if err != nil {
		return nil, err
	}

	return []string{keepPath, genericPath}, nil
}

func (s *ServiceAccountStore) Existing(email string, includeKeepFallback bool) (ServiceAccountFile, bool, error) {
	if err := s.validateLayout(true); err != nil {
		return ServiceAccountFile{}, false, err
	}

	candidates := s.candidates(email, includeKeepFallback)
	if len(candidates) == 0 {
		return ServiceAccountFile{}, false, nil
	}

	for _, candidate := range candidates {
		info, err := os.Stat(candidate.Path)
		if err == nil {
			candidate.ModifiedAt = info.ModTime()
			return candidate, true, nil
		}

		if !os.IsNotExist(err) {
			return ServiceAccountFile{}, false, fmt.Errorf("stat service account path: %w", err)
		}
	}

	return candidates[0], false, nil
}

func (s *ServiceAccountStore) Read(email string, includeKeepFallback bool) (ServiceAccountFile, bool, error) {
	file, exists, err := s.Existing(email, includeKeepFallback)
	if err != nil || !exists {
		return file, exists, err
	}

	data, err := os.ReadFile(file.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return ServiceAccountFile{
				Path:         file.Path,
				KeepSpecific: file.KeepSpecific,
			}, false, nil
		}

		return ServiceAccountFile{}, false, fmt.Errorf("read service account key: %w", err)
	}

	data, err = s.ResolveKey(data)
	if err != nil {
		return ServiceAccountFile{}, false, err
	}

	file.Data = data

	return file, true, nil
}

func (s *ServiceAccountStore) Remove(email string) (bool, error) {
	if err := s.validateLayout(true); err != nil {
		return false, err
	}

	candidates := s.candidates(email, true)
	removed := false

	for _, candidate := range candidates {
		if err := s.removeKeyringCopy(candidate.Path); err != nil {
			return removed, err
		}

		if err := os.Remove(candidate.Path); err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return removed, fmt.Errorf("remove service account file: %w", err)
		}
		removed = true
	}

	return removed, nil
}

func (s *ServiceAccountStore) ListEmails() ([]string, error) {
	if err := s.validateLayout(true); err != nil {
		return nil, err
	}

	out := make([]string, 0)
	seen := make(map[string]struct{})

	dirs := []string{s.layout.DataDir}
	if !s.layout.ExplicitData {
		dirs = append(dirs, s.layout.ConfigDir)
	}

	for _, dir := range uniquePaths(dirs...) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}

			return nil, fmt.Errorf("read service account dir: %w", err)
		}

		for _, entry := range entries {
			email := serviceAccountEmailFromEntry(entry)
			if email == "" {
				continue
			}

			if _, ok := seen[email]; ok {
				continue
			}
			seen[email] = struct{}{}
			out = append(out, email)
		}
	}

	sort.Strings(out)

	return out, nil
}

func (s *ServiceAccountStore) secrets() (ServiceAccountSecrets, error) {
	if s.openSecrets == nil {
		return nil, errServiceAccountSecretsMissing
	}

	store, err := s.openSecrets()
	if err != nil {
		return nil, fmt.Errorf("open keyring: %w", err)
	}

	if store == nil {
		return nil, errServiceAccountSecretsMissing
	}

	return store, nil
}

// ResolveKey returns the key JSON for the contents of a service account file: the contents themselves,
// or the key in the keyring when the file holds only a reference to it.
func (s *ServiceAccountStore) ResolveKey(data []byte) ([]byte, error) {
	key, ok := keyringKeyFromServiceAccountFile(data)
	if !ok {
		return data, nil
	}

	store, err := s.secrets()
	if err != nil {
		return nil, err
	}

	data, err = store.GetSecret(key)
	if err != nil {
		return nil, fmt.Errorf("read service account key from keyring: %w", err)
	}

	return data, nil
}

// IsServiceAccountKeyringReference reports whether service account file contents are a keyring reference.
func IsServiceAccountKeyringReference(data []byte) bool {
	_, ok := keyringKeyFromServiceAccountFile(data)

	return ok
}

// keyringKeyAt returns the keyring key named by the file at path; a missing or unreadable file names none.
func (s *ServiceAccountStore) keyringKeyAt(path string) (string, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is one of the store's own candidates
	if err != nil {
		return "", false
	}

	return keyringKeyFromServiceAccountFile(data)
}

func (s *ServiceAccountStore) removeKeyringCopy(path string) error {
	key, ok := s.keyringKeyAt(path)
	if !ok {
		return nil
	}

	return s.deleteKeyringKey(key)
}

func (s *ServiceAccountStore) deleteKeyringKey(key string) error {
	store, err := s.secrets()
	if err != nil {
		return err
	}

	if err := store.DeleteSecret(key); err != nil && !errors.Is(err, keyring.ErrKeyNotFound) {
		return fmt.Errorf("delete service account key from keyring: %w", err)
	}

	return nil
}

func keyringKeyFromServiceAccountFile(data []byte) (string, bool) {
	var ref serviceAccountKeyringRef
	if err := json.Unmarshal(data, &ref); err != nil {
		return "", false
	}

	key := strings.TrimSpace(ref.KeyringKey)

	return key, strings.HasPrefix(key, serviceAccountKeyringKeyPrefix)
}

func (s *ServiceAccountStore) ensureDataDir() error {
	if err := os.MkdirAll(s.layout.DataDir, 0o700); err != nil {
		return fmt.Errorf("ensure data dir: %w", err)
	}

	return nil
}

func (s *ServiceAccountStore) validateLayout(includeLegacy bool) error {
	if s == nil || strings.TrimSpace(s.layout.DataDir) == "" {
		return fmt.Errorf("%w: data directory is required", errIncompleteServiceAccountLayout)
	}

	if includeLegacy && !s.layout.ExplicitData && strings.TrimSpace(s.layout.ConfigDir) == "" {
		return fmt.Errorf("%w: config directory is required when legacy lookup is enabled", errIncompleteServiceAccountLayout)
	}

	return nil
}

func (s *ServiceAccountStore) candidates(email string, includeKeepFallback bool) []ServiceAccountFile {
	candidates := []ServiceAccountFile{{Path: s.layout.ServiceAccountPath(email)}}
	if !s.layout.ExplicitData {
		candidates = append(candidates, ServiceAccountFile{Path: s.layout.ServiceAccountLegacyPath(email)})
	}

	if !includeKeepFallback {
		return uniqueServiceAccountFiles(candidates)
	}

	candidates = append(candidates, ServiceAccountFile{
		Path:         s.layout.KeepServiceAccountPath(email),
		KeepSpecific: true,
	})
	if !s.layout.ExplicitData {
		candidates = append(candidates, ServiceAccountFile{
			Path:         s.layout.KeepServiceAccountLegacySafePath(email),
			KeepSpecific: true,
		})
		if path, ok := s.rawKeepLegacyPath(email); ok {
			candidates = append(candidates, ServiceAccountFile{
				Path:         path,
				KeepSpecific: true,
			})
		}
	}

	return uniqueServiceAccountFiles(candidates)
}

func (s *ServiceAccountStore) rawKeepLegacyPath(email string) (string, bool) {
	if strings.ContainsAny(email, `/\`) {
		return "", false
	}

	path := filepath.Clean(s.layout.KeepServiceAccountLegacyPath(email))

	base := filepath.Base(path)
	if filepath.Dir(path) != filepath.Clean(s.layout.ConfigDir) ||
		!strings.HasPrefix(base, "keep-sa-") ||
		!strings.HasSuffix(base, ".json") {
		return "", false
	}

	return path, true
}

func uniqueServiceAccountFiles(files []ServiceAccountFile) []ServiceAccountFile {
	out := make([]ServiceAccountFile, 0, len(files))
	seen := make(map[string]struct{}, len(files))

	for _, file := range files {
		if file.Path == "" {
			continue
		}

		clean := filepath.Clean(file.Path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		file.Path = clean
		out = append(out, file)
	}

	return out
}

func (l Layout) EnsureDataDir() (string, error) {
	if err := os.MkdirAll(l.DataDir, 0o700); err != nil {
		return "", fmt.Errorf("ensure data dir: %w", err)
	}

	return l.DataDir, nil
}

func serviceAccountEmailFromEntry(entry os.DirEntry) string {
	if entry.IsDir() {
		return ""
	}

	name := entry.Name()
	var encoded string

	switch {
	case strings.HasPrefix(name, "sa-") && strings.HasSuffix(name, ".json"):
		encoded = strings.TrimSuffix(strings.TrimPrefix(name, "sa-"), ".json")
	case strings.HasPrefix(name, "keep-sa-") && strings.HasSuffix(name, ".json"):
		encoded = strings.TrimSuffix(strings.TrimPrefix(name, "keep-sa-"), ".json")
	default:
		return ""
	}

	if decoded, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
		return strings.ToLower(strings.TrimSpace(string(decoded)))
	}

	if strings.HasPrefix(name, "keep-sa-") {
		return strings.ToLower(strings.TrimSpace(encoded))
	}

	return ""
}
