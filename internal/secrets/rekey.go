package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/99designs/keyring"
	jose "github.com/dvsekhvalnov/jose2go"
	"github.com/mtibben/percent"

	"github.com/openclaw/gogcli/internal/config"
)

const keyringBackendFile = "file"

var (
	ErrRekeyNotFileBackend  = errors.New("rekey requires the file keyring backend")
	ErrRekeySamePassword    = errors.New("new password must differ from the current password")
	ErrRekeyNoOldPassword   = errors.New("no current keyring password provided; set GOG_KEYRING_PASSWORD or run on a TTY")
	ErrRekeyNoNewPassword   = errors.New("no new password provided; set GOG_KEYRING_NEW_PASSWORD or run on a TTY")
	errRekeyUnreadableItems = errors.New("keyring items cannot be decrypted with the current password")
	errRekeyNoReadableItems = errors.New("the current password does not open any keyring item")
	errRekeyContentMismatch = errors.New("keyring content mismatch")
	errRekeyUnsafeOutput    = errors.New("output directory overlaps the live keyring")
	errRekeyOutputNotEmpty  = errors.New("output directory is not empty; use --force to replace it")
	errRekeyNoOutput        = errors.New("output directory is required; pass it as the positional argument")
)

// RekeyOptions describes an output-only keyring password change.
type RekeyOptions struct {
	Layout         config.Layout
	Backend        string
	OldPassword    string
	OldPasswordSet bool
	NewPassword    string
	NewPasswordSet bool
	GOOS           string
	DBusAddress    string
	// OutDir is the required output directory, passed positionally on the
	// command line. Force permits replacing its contents.
	OutDir         string
	Force          bool
	SkipUnreadable bool
	// openOutput is a test seam for write and independent read-back failures.
	openOutput func(dir, password string) (keyring.Keyring, error)
}

// RekeyResult reports what was written, never a change to the live keyring.
type RekeyResult struct {
	Dir         string
	OutDir      string
	Items       int
	Keys        []string
	Unreadable  []string
	Replaced    []string
	MoveCommand string
	// RetainedDir is where the printed installation command would keep the old
	// directory. Rekey itself never creates or moves anything there.
	RetainedDir string
	// Deprecated: the live password never changes and no backup is made.
	BackupDir       string
	PasswordChanged bool
}

// Rekey opens the live keyring read-only and writes re-encrypted entries only to
// OutDir. Nothing the live directory holds is ever modified, so no crash,
// interruption or bug in output writing can lose or corrupt live credentials.
// Re-running with --force rewrites the output directory from the live store.
func Rekey(opts RekeyOptions) (RekeyResult, error) {
	if !opts.OldPasswordSet {
		return RekeyResult{}, ErrRekeyNoOldPassword
	}

	if !opts.NewPasswordSet {
		return RekeyResult{}, ErrRekeyNoNewPassword
	}

	if opts.OldPassword == opts.NewPassword {
		return RekeyResult{}, ErrRekeySamePassword
	}

	if err := CheckRekeyBackend(opts.Backend, opts.GOOS, opts.DBusAddress); err != nil {
		return RekeyResult{}, err
	}

	if opts.OutDir == "" {
		return RekeyResult{}, errRekeyNoOutput
	}

	dir, err := filepath.EvalSymlinks(opts.Layout.KeyringDir())
	if err != nil {
		return RekeyResult{}, fmt.Errorf("resolve live keyring: %w", err)
	}

	dir, err = filepath.Abs(dir)
	if err != nil {
		return RekeyResult{}, fmt.Errorf("resolve live keyring path: %w", err)
	}

	out, err := rekeyOutputPath(dir, opts.OutDir)
	if err != nil {
		return RekeyResult{}, err
	}

	info, err := os.Stat(dir)
	if err != nil {
		return RekeyResult{}, fmt.Errorf("stat live keyring: %w", err)
	}

	items, unreadable, err := readRekeyItems(dir, opts.OldPassword, opts.SkipUnreadable)
	if err != nil {
		return RekeyResult{}, err
	}

	mode := info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	replaced, err := prepareRekeyOutput(out, mode, opts.Force)

	res := RekeyResult{Dir: dir, OutDir: out, Unreadable: unreadable, Replaced: replaced}
	if err != nil {
		return res, err
	}

	open := opts.openOutput
	if open == nil {
		open = openRekeyOutput
	}

	dst, err := open(out, opts.NewPassword)
	if err != nil {
		return res, fmt.Errorf("open output keyring: %w", err)
	}

	if err = writeRekeyItems(dst, items); err != nil {
		return res, err
	}
	// Reopen independently with the NEW password. Comparing the whole item also
	// protects metadata, not just the credential data. Never print decoded data.
	res.Keys, err = verifyRekeyOutput(open, out, opts.NewPassword, items)
	if err != nil {
		return res, err
	}
	res.Items = len(items)
	res.RetainedDir = rekeyRetainedDir(dir, out)
	res.MoveCommand = rekeyMoveCommand(dir, out)

	return res, nil
}

func writeRekeyItems(dst keyring.Keyring, items []keyring.Item) error {
	for _, item := range items {
		if err := dst.Set(item); err != nil {
			return fmt.Errorf("write output item %q: %w", item.Key, err)
		}
	}

	return nil
}

func verifyRekeyOutput(open func(string, string) (keyring.Keyring, error), out, password string, items []keyring.Item) ([]string, error) {
	verify, err := open(out, password)
	if err != nil {
		return nil, fmt.Errorf("open output for verification: %w", err)
	}

	keys := make([]string, 0, len(items))
	for _, item := range items {
		got, getErr := verify.Get(item.Key)
		if getErr != nil || !reflect.DeepEqual(got, item) {
			return nil, fmt.Errorf("%w: output item %q failed verification", errRekeyContentMismatch, item.Key)
		}

		keys = append(keys, item.Key)
	}

	listed, err := verify.Keys()
	if err != nil {
		return nil, fmt.Errorf("list output for verification: %w", err)
	}

	slices.Sort(listed)

	if !slices.Equal(listed, keys) {
		return nil, fmt.Errorf("%w: output keys differ", errRekeyContentMismatch)
	}

	return keys, nil
}

// readRekeyItems deliberately bypasses the normal keyring opener: both it and
// the upstream file backend can mkdir on reads, and Store takes a lock that can
// create a file. Only ReadDir, Lstat and ReadFile touch the source here.
func readRekeyItems(dir, password string, skip bool) ([]keyring.Item, []string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read live keyring directory: %w", err)
	}
	items := make(map[string]keyring.Item)
	var unreadable []string

	for _, entry := range entries {
		key := decodeFileSafeKey(percent.Decode(entry.Name()))
		if entry.IsDir() || isInternalKeyringKey(key) || entry.Name() == ".DS_Store" {
			continue
		}

		info, statErr := os.Lstat(filepath.Join(dir, entry.Name()))
		if statErr != nil {
			return nil, nil, fmt.Errorf("stat live item %q: %w", key, statErr)
		}

		if !info.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("%w: nonregular live item %q", errRekeyUnreadableItems, key)
		}

		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // name from ReadDir, source read-only
		if readErr != nil {
			return nil, nil, fmt.Errorf("read live item %q: %w", key, readErr)
		}
		payload, _, decodeErr := jose.Decode(string(data), password)

		var item keyring.Item
		if decodeErr != nil || json.Unmarshal([]byte(payload), &item) != nil {
			unreadable = append(unreadable, key)
			continue
		}

		item.Key = key
		if previous, ok := items[key]; ok && !reflect.DeepEqual(previous, item) {
			return nil, nil, fmt.Errorf("%w: conflicting live copies of %q", errRekeyContentMismatch, key)
		}
		items[key] = item
	}

	slices.Sort(unreadable)

	if len(unreadable) > 0 {
		if len(items) == 0 {
			return nil, unreadable, fmt.Errorf("%w: %s", errRekeyNoReadableItems, strings.Join(unreadable, ", "))
		}

		if !skip {
			return nil, unreadable, fmt.Errorf("%w: %s", errRekeyUnreadableItems, strings.Join(unreadable, ", "))
		}
	}

	result := make([]keyring.Item, 0, len(items))
	for _, item := range items {
		result = append(result, item)
	}

	slices.SortFunc(result, func(a, b keyring.Item) int { return strings.Compare(a.Key, b.Key) })

	return result, unreadable, nil
}

// Resolve existing parents too, so aliases and paths through symlinks cannot
// point output into the source. Reject ancestors as well: --force on one would
// remove the live directory. A final symlink is never an output directory.
func rekeyOutputPath(dir, out string) (string, error) {
	absolute, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("resolve output path: %w", err)
	}

	if info, statErr := os.Lstat(absolute); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: output is a symlink: %s", errRekeyUnsafeOutput, absolute)
	}

	resolved, err := resolveRekeyParents(absolute)
	if err != nil {
		return "", err
	}

	if rekeyPathContains(dir, resolved) || rekeyPathContains(resolved, dir) {
		return "", fmt.Errorf("%w: %s and %s", errRekeyUnsafeOutput, dir, resolved)
	}

	return resolved, nil
}

func resolveRekeyParents(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}

	parent := filepath.Dir(path)
	if parent == path {
		return "", fmt.Errorf("resolve output parent: %w", err)
	}

	resolved, err = resolveRekeyParents(parent)
	if err != nil {
		return "", err
	}

	return filepath.Join(resolved, filepath.Base(path)), nil
}

func rekeyPathContains(parent, child string) bool {
	// Lexical checks alone miss case aliases on macOS's usual filesystem and
	// Windows short names. Compare directory identities along existing parents
	// as well, including when the requested output does not exist yet.
	parentInfo, statErr := os.Stat(parent)
	if statErr == nil {
		for path := child; ; path = filepath.Dir(path) {
			if info, err := os.Stat(path); err == nil && os.SameFile(parentInfo, info) {
				return true
			}

			if filepath.Dir(path) == path {
				break
			}
		}
	}

	if runtime.GOOS == "windows" {
		parent, child = strings.ToLower(parent), strings.ToLower(child)
	}
	rel, err := filepath.Rel(parent, child)

	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func prepareRekeyOutput(out string, mode os.FileMode, force bool) ([]string, error) {
	return prepareRekeyOutputWithRemove(out, mode, force, os.RemoveAll)
}

// Removing entries individually lets a failed replacement report exactly which
// top-level entries were removed. The directory itself is also recreated; no
// contents from an earlier run are merged into the new output.
func prepareRekeyOutputWithRemove(out string, mode os.FileMode, force bool, remove func(string) error) ([]string, error) {
	entries, err := os.ReadDir(out)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read output directory: %w", err)
	}
	var replaced []string

	if len(entries) > 0 {
		if !force {
			return nil, errRekeyOutputNotEmpty
		}

		for _, entry := range entries {
			if err := remove(filepath.Join(out, entry.Name())); err != nil {
				return replaced, fmt.Errorf("replace output entry %q (it may be partially removed): %w", entry.Name(), err)
			}
			replaced = append(replaced, entry.Name())
		}

		if err := remove(out); err != nil {
			return replaced, fmt.Errorf("replace output directory: %w", err)
		}
	}

	if err := os.MkdirAll(out, mode); err != nil {
		return replaced, fmt.Errorf("create output directory: %w", err)
	}
	// MkdirAll honors umask and leaves existing empty directories unchanged.
	if err := os.Chmod(out, mode); err != nil {
		return replaced, fmt.Errorf("match output directory permissions: %w", err)
	}

	return replaced, nil
}

func openRekeyOutput(dir, password string) (keyring.Keyring, error) {
	ring, err := keyring.Open(keyring.Config{
		ServiceName:      config.AppName,
		AllowedBackends:  []keyring.BackendType{keyring.FileBackend},
		FileDir:          dir,
		FilePasswordFunc: keyring.FixedStringPrompt(password),
	})
	if err != nil {
		return nil, fmt.Errorf("open file keyring: %w", err)
	}

	return newFileSafeKeyringIn(ring, dir), nil
}

func shellQuote(path string) string { return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'" }

func rekeyMoveCommand(dir, out string) string {
	old := rekeyRetainedDir(dir, out)

	if runtime.GOOS == "windows" {
		quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
		return "if (Get-Item -Force -LiteralPath " + quote(old) + " -ErrorAction SilentlyContinue) { throw 'Old directory already exists' }; Move-Item -LiteralPath " + quote(dir) + " -Destination " + quote(old) + " -ErrorAction Stop; Move-Item -LiteralPath " + quote(out) + " -Destination " + quote(dir) + " -ErrorAction Stop"
	}

	return "test ! -e " + shellQuote(old) + " && test ! -L " + shellQuote(old) + " && mv -- " + shellQuote(dir) + " " + shellQuote(old) + " && mv -- " + shellQuote(out) + " " + shellQuote(dir)
}

func rekeyRetainedDir(dir, out string) string {
	old := dir + ".old"
	for suffix := 1; rekeyPathContains(old, out) || rekeyPathContains(out, old); suffix++ {
		old = fmt.Sprintf("%s.old-%d", dir, suffix)
	}

	return old
}

// CheckRekeyBackend rejects OS keychains without opening or creating any store.
// auto qualifies only for the headless Linux configuration that forces file.
func CheckRekeyBackend(backend, goos, dbusAddress string) error {
	if rekeyUsesFileBackend(RekeyOptions{Backend: backend, GOOS: goos, DBusAddress: dbusAddress}) {
		return nil
	}

	return fmt.Errorf("%w (current backend: %s)", ErrRekeyNotFileBackend, rekeyBackendLabel(backend))
}

func rekeyUsesFileBackend(opts RekeyOptions) bool {
	value := strings.ToLower(strings.TrimSpace(opts.Backend))
	return value == keyringBackendFile || shouldForceFileBackend(opts.GOOS, KeyringBackendInfo{Value: value}, opts.DBusAddress)
}

func rekeyBackendLabel(backend string) string {
	if label := strings.TrimSpace(backend); label != "" {
		return label
	}

	return "unresolved"
}
