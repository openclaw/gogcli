package secrets

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/99designs/keyring"
)

// isInvalidFilenameError reports whether err is the platform-specific signal
// that the underlying filesystem rejected a path because the filename
// contains characters it considers illegal (e.g. NTFS rejecting ":" via
// ERROR_INVALID_NAME on Windows). Tests swap this to simulate other
// platforms; the runtime default is wired up per-OS via
// filename_safe_{windows,other}.go.
var isInvalidFilenameError = defaultIsInvalidFilenameError

// isNotFound treats keyring.ErrKeyNotFound, fs.ErrNotExist, and the
// platform-specific "filename invalid" signal as "not found". The third case
// matters on Windows: the 99designs/keyring file backend forwards
// ERROR_INVALID_NAME from os.OpenFile/os.Remove rather than translating to
// the keyring sentinel, so a raw-key fallback for a path with NTFS-illegal
// characters would otherwise look like a hard error to callers branching on
// ErrKeyNotFound.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, keyring.ErrKeyNotFound) || errors.Is(err, fs.ErrNotExist) {
		return true
	}

	return isInvalidFilenameError(err)
}

// safeFilenameMap lists characters that are illegal in NTFS / Windows
// filenames plus "%" itself, so percent-style encoding is self-inverse. The
// 99designs/keyring file backend already escapes "/" via percent.Encode, so
// it is omitted.
//
// Order matters: "%" -> "%25" must run first on encode and last on decode.
//
// Not handled here, on purpose: Windows reserved device names (CON, PRN,
// AUX, NUL, COM1-9, LPT1-9), trailing spaces / dots, and ASCII control
// characters. The gogcli key namespace ("token:<client>:<email>",
// "default_account[:<client>]") cannot produce any of those forms; if a
// future caller wires arbitrary user input through SetSecret, this list is
// the place to extend.
var safeFilenameMap = []struct {
	raw     string
	encoded string
}{
	{`%`, "%25"},
	{`:`, "%3A"},
	{`<`, "%3C"},
	{`>`, "%3E"},
	{`"`, "%22"},
	{`|`, "%7C"},
	{`?`, "%3F"},
	{`*`, "%2A"},
	{`\`, "%5C"},
}

// encodedKeyPrefix marks item keys that fileSafeKeyring rewrote so Keys()
// can distinguish wrapper-written entries from legacy raw-form files. Older
// gogcli versions wrote keys directly (e.g. "token:default:foo@bar.com"),
// none of which start with this sentinel.
//
// Choosing a printable, filesystem-safe sentinel (vs. percent-encoded NULs
// or high-bit Unicode) keeps the on-disk filenames human-debuggable on every
// platform.
const encodedKeyPrefix = "_v1_"

// needsFilenameEncoding reports whether s contains any character that the
// file keyring backend would persist as an illegal NTFS filename byte, or
// whether it collides with the encodedKeyPrefix sentinel and therefore must
// be encoded to round-trip cleanly through Keys() / decodeFilenameKey.
func needsFilenameEncoding(s string) bool {
	if strings.HasPrefix(s, encodedKeyPrefix) {
		return true
	}

	for _, p := range safeFilenameMap {
		if strings.Contains(s, p.raw) {
			return true
		}
	}

	return false
}

// encodeFilenameKey percent-encodes Windows-illegal characters in key and
// prepends encodedKeyPrefix so fileSafeKeyring can recognise its own writes
// later. Keys with no encodable character pass through unchanged so existing
// macOS/Linux keyring directories remain readable without manual migration.
func encodeFilenameKey(key string) string {
	if !needsFilenameEncoding(key) {
		return key
	}

	out := key
	for _, p := range safeFilenameMap {
		out = strings.ReplaceAll(out, p.raw, p.encoded)
	}

	return encodedKeyPrefix + out
}

// decodeFilenameKey reverses encodeFilenameKey. Names lacking
// encodedKeyPrefix are treated as legacy raw-form keys and returned
// unchanged, which avoids corrupting any pre-existing item whose key
// happened to contain a literal "%XX" sequence.
func decodeFilenameKey(name string) string {
	if !strings.HasPrefix(name, encodedKeyPrefix) {
		return name
	}

	out := strings.TrimPrefix(name, encodedKeyPrefix)

	for i := len(safeFilenameMap) - 1; i >= 0; i-- {
		p := safeFilenameMap[i]
		out = strings.ReplaceAll(out, p.encoded, p.raw)
	}

	return out
}

// fileSafeKeyring wraps a keyring.Keyring (intended for the file backend) and
// percent-encodes NTFS-illegal characters in item keys so filenames are
// portable across operating systems.
//
// Reads fall back to the raw (un-encoded) key form so file-keyring directories
// written by older gogcli versions on macOS/Linux keep working without a
// manual migration. Writes opportunistically remove the legacy file once the
// new encoded form has been persisted.
type fileSafeKeyring struct {
	inner keyring.Keyring
}

func newFileSafeKeyring(inner keyring.Keyring) *fileSafeKeyring {
	return &fileSafeKeyring{inner: inner}
}

func (k *fileSafeKeyring) Get(key string) (keyring.Item, error) {
	encoded := encodeFilenameKey(key)
	if encoded != key {
		if it, err := k.inner.Get(encoded); err == nil {
			it.Key = key

			return it, nil
		} else if !isNotFound(err) {
			return keyring.Item{}, fmt.Errorf("file keyring get: %w", err)
		}
	}

	it, err := k.inner.Get(key)
	if err != nil {
		if isNotFound(err) {
			return keyring.Item{}, keyring.ErrKeyNotFound
		}

		return keyring.Item{}, fmt.Errorf("file keyring get: %w", err)
	}

	it.Key = key

	return it, nil
}

func (k *fileSafeKeyring) GetMetadata(key string) (keyring.Metadata, error) {
	encoded := encodeFilenameKey(key)
	if encoded != key {
		if md, err := k.inner.GetMetadata(encoded); err == nil {
			return md, nil
		} else if !isNotFound(err) {
			return keyring.Metadata{}, fmt.Errorf("file keyring metadata: %w", err)
		}
	}

	md, err := k.inner.GetMetadata(key)
	if err != nil {
		if isNotFound(err) {
			return keyring.Metadata{}, keyring.ErrKeyNotFound
		}

		return keyring.Metadata{}, fmt.Errorf("file keyring metadata: %w", err)
	}

	return md, nil
}

func (k *fileSafeKeyring) Set(item keyring.Item) error {
	encoded := encodeFilenameKey(item.Key)

	stored := item
	stored.Key = encoded

	if err := k.inner.Set(stored); err != nil {
		return fmt.Errorf("file keyring set: %w", err)
	}

	// Best-effort: remove a legacy raw-key file written by older versions so
	// Keys() does not return duplicates after the in-place migration. Failure
	// is non-fatal — a subsequent Set/Remove will retry. isNotFound also
	// classifies the platform's "filename invalid" error as not-found, so on
	// Windows + file backend (where the legacy filename can't exist anyway)
	// the call collapses harmlessly.
	if encoded != item.Key {
		if err := k.inner.Remove(item.Key); err != nil && !isNotFound(err) {
			_ = err
		}
	}

	return nil
}

func (k *fileSafeKeyring) Remove(key string) error {
	encoded := encodeFilenameKey(key)

	encErr := k.inner.Remove(encoded)
	if encErr != nil && !isNotFound(encErr) {
		return fmt.Errorf("file keyring remove: %w", encErr)
	}

	encMissing := isNotFound(encErr)

	if encoded == key {
		if encMissing {
			return keyring.ErrKeyNotFound
		}

		return nil
	}

	// Try legacy raw-form cleanup. isNotFound treats both
	// keyring.ErrKeyNotFound and the platform "filename invalid" signal
	// (Windows ERROR_INVALID_NAME for NTFS-illegal paths) as "nothing to
	// clean up", so this call collapses harmlessly when the legacy file
	// could never have existed on the underlying filesystem.
	rawErr := k.inner.Remove(key)
	if rawErr != nil && !isNotFound(rawErr) {
		return fmt.Errorf("file keyring remove: %w", rawErr)
	}

	if encMissing && isNotFound(rawErr) {
		return keyring.ErrKeyNotFound
	}

	return nil
}

func (k *fileSafeKeyring) Keys() ([]string, error) {
	raw, err := k.inner.Keys()
	if err != nil {
		return nil, fmt.Errorf("file keyring list: %w", err)
	}

	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))

	for _, name := range raw {
		decoded := decodeFilenameKey(name)
		if _, ok := seen[decoded]; ok {
			continue
		}

		seen[decoded] = struct{}{}

		out = append(out, decoded)
	}

	return out, nil
}
