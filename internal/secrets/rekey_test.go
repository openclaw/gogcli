package secrets

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/99designs/keyring"

	"github.com/openclaw/gogcli/internal/config"
)

func newRekeyTestLayout(tb testing.TB) (config.Layout, string) {
	tb.Helper()

	layout := config.Layout{DataDir: tb.TempDir(), ExplicitData: true}

	dir := layout.KeyringDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tb.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, keyringLockFilename), nil, 0o600); err != nil {
		tb.Fatal(err)
	}
	return layout, dir
}

func openFileRing(tb testing.TB, dir, password string) keyring.Keyring {
	tb.Helper()

	ring, err := openRekeyOutput(dir, password)
	if err != nil {
		tb.Fatal(err)
	}

	return ring
}

func rekeyFixture(t *testing.T) (RekeyOptions, string) {
	t.Helper()
	layout, dir := newRekeyTestLayout(t)

	item := keyring.Item{Key: "credential/one", Data: []byte("private-payload"), Label: "label", Description: "description"}
	if err := openFileRing(t, dir, "old").Set(item); err != nil {
		t.Fatal(err)
	}

	return RekeyOptions{Layout: layout, Backend: "file", GOOS: runtime.GOOS, OldPassword: "old", OldPasswordSet: true, NewPassword: "new", NewPasswordSet: true, OutDir: filepath.Join(t.TempDir(), "output")}, dir
}

type rekeySnapshotEntry struct {
	Mode os.FileMode
	Data string
}

// Snapshot all files, nested directories, symlink targets and modes, including
// bookkeeping. Absence of any newly-created lock or verifier is part of equality.
func snapshotRekeyDir(tb testing.TB, dir string) map[string]rekeySnapshotEntry {
	tb.Helper()

	result := make(map[string]rekeySnapshotEntry)

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", path, err)
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("info %q: %w", path, err)
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("rel %q: %w", path, err)
		}

		var data []byte
		if info.Mode().IsRegular() {
			data, err = os.ReadFile(path)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			var target string
			target, err = os.Readlink(path)
			data = []byte(target)
		}

		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}

		result[rel] = rekeySnapshotEntry{Mode: info.Mode(), Data: string(data)}

		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}

	return result
}

func TestRekey_LiveByteIdentical(t *testing.T) {
	for _, scenario := range []string{"success", "wrong-password", "write-failure", "verify-failure", "nonempty", "unsafe"} {
		t.Run(scenario, func(t *testing.T) {
			opts, dir := rekeyFixture(t)
			if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
				t.Fatal(err)
			}

			if err := os.WriteFile(filepath.Join(dir, "nested", "note"), []byte("untouched"), 0o600); err != nil {
				t.Fatal(err)
			}

			switch scenario {
			case "wrong-password":
				opts.OldPassword = "wrong"
			case "write-failure":
				opts.openOutput = func(dir, password string) (keyring.Keyring, error) {
					return &rekeyFaultRing{Keyring: openFileRing(t, dir, password), failSet: true}, nil
				}
			case "verify-failure":
				opts.openOutput = func(dir, password string) (keyring.Keyring, error) {
					return &rekeyFaultRing{Keyring: openFileRing(t, dir, password), corruptGet: true}, nil
				}
			case "nonempty":
				opts.OutDir = t.TempDir()
				if err := os.WriteFile(filepath.Join(opts.OutDir, "sentinel"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "unsafe":
				opts.OutDir = dir
				opts.Force = true
			}
			before := snapshotRekeyDir(t, dir)

			_, err := Rekey(opts)
			if (err == nil) != (scenario == "success") {
				t.Fatalf("unexpected result: %v", err)
			}

			if after := snapshotRekeyDir(t, dir); !reflect.DeepEqual(before, after) {
				t.Fatal("live directory changed")
			}
		})
	}
}

func TestRekey_InterruptedLiveByteIdentical(t *testing.T) {
	const childEnv = "GOG_REKEY_INTERRUPTION_TEST_DIR"
	if dir := os.Getenv(childEnv); dir != "" {
		opts := RekeyOptions{
			Layout:  config.Layout{DataDir: dir, ExplicitData: true},
			Backend: "file", GOOS: runtime.GOOS,
			OldPassword: "old", OldPasswordSet: true,
			NewPassword: "new", NewPasswordSet: true,
			OutDir: filepath.Join(dir, "output"),
		}
		opens := 0

		opts.openOutput = func(dir, password string) (keyring.Keyring, error) {
			opens++
			if opens == 2 {
				// Output has been written, but verification has not begun.
				fmt.Println("output written")

				for {
					time.Sleep(time.Hour)
				}
			}

			return openRekeyOutput(dir, password)
		}
		if _, err := Rekey(opts); err != nil {
			t.Fatal(err)
		}

		t.Fatal("child returned without interruption")
	}

	opts, dir := rekeyFixture(t)
	before := snapshotRekeyDir(t, dir)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestRekey_InterruptedLiveByteIdentical$")

	child.Env = append(os.Environ(), childEnv+"="+opts.Layout.DataDir)

	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()

	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "output written\n" {
		t.Fatalf("child did not reach interruption point: %q %v", line, err)
	}

	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}

	if err := child.Wait(); err == nil {
		t.Fatal("interrupted process reported success")
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("interruption changed live directory")
	}
}

func TestRekey_OutputRoundTrip(t *testing.T) {
	opts, dir := rekeyFixture(t)
	// Every kind of key, including names illegal as raw Windows filenames, is
	// stored encoded. This creates no invalid filenames on any platform.
	src := openFileRing(t, dir, "old")
	for _, key := range []string{"token:default:a@example.com", "default-account", ".credential", ".DS_Store"} {
		if err := src.Set(keyring.Item{Key: key, Data: []byte(key), Label: "metadata"}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := Rekey(opts)
	if err != nil {
		t.Fatal(err)
	}

	wantKeys := []string{".DS_Store", ".credential", "credential/one", "default-account", "token:default:a@example.com"}
	// Do not trust the returned keys as the round-trip inputs: the legacy
	// fallback can read an encoded filename as though it were a raw key, masking
	// a missing decode step and a second layer of encoding in the output.
	if res.Items != len(wantKeys) || !reflect.DeepEqual(res.Keys, wantKeys) || res.PasswordChanged || res.BackupDir != "" {
		t.Fatalf("incorrect result: %+v", res)
	}

	for _, key := range wantKeys {
		want, wantErr := src.Get(key)
		if wantErr != nil {
			t.Fatal(wantErr)
		}

		got, gotErr := openFileRing(t, opts.OutDir, "new").Get(key)
		if gotErr != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("item %q failed round trip: %v", key, gotErr)
		}

		if _, oldErr := openFileRing(t, opts.OutDir, "old").Get(key); oldErr == nil {
			t.Fatal("output accepts old password")
		}
	}

	entries, err := os.ReadDir(opts.OutDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 5 {
		t.Fatalf("unexpected output entries: %d", len(entries))
	}
}

func TestRekey_Guards(t *testing.T) {
	for _, scenario := range []string{"missing-old", "missing-new", "same", "backend", "missing-output", "nonempty", "same-dir", "inside", "ancestor", "wrong", "all-skipped"} {
		t.Run(scenario, func(t *testing.T) {
			opts, dir := rekeyFixture(t)
			want := ErrRekeyNoOldPassword

			switch scenario {
			case "missing-old":
				opts.OldPasswordSet = false
			case "missing-new":
				opts.NewPasswordSet = false
				want = ErrRekeyNoNewPassword
			case "same":
				opts.NewPassword = opts.OldPassword
				want = ErrRekeySamePassword
			case "backend":
				opts.Backend = "keychain"
				want = ErrRekeyNotFileBackend
			case "missing-output":
				opts.OutDir = ""
				// Removing the required-output guard must reach a fresh empty
				// directory, rather than fail on unrelated files in the checkout.
				t.Chdir(t.TempDir())
				want = errRekeyNoOutput
			case "nonempty":
				opts.OutDir = t.TempDir()
				if err := os.WriteFile(filepath.Join(opts.OutDir, "keep"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
				want = errRekeyOutputNotEmpty
			case "same-dir":
				opts.OutDir = dir
				opts.Force = true
				want = errRekeyUnsafeOutput
			case "inside":
				opts.OutDir = filepath.Join(dir, "output")
				opts.Force = true
				want = errRekeyUnsafeOutput
			case "ancestor":
				opts.OutDir = filepath.Dir(dir)
				opts.Force = true
				want = errRekeyUnsafeOutput
			case "wrong":
				opts.OldPassword = "wrong"
				want = errRekeyNoReadableItems
			case "all-skipped":
				opts.OldPassword = "wrong"
				opts.SkipUnreadable = true
				want = errRekeyNoReadableItems
			}
			before := snapshotRekeyDir(t, dir)

			_, err := Rekey(opts)
			if !errors.Is(err, want) {
				t.Fatalf("want %v, got %v", want, err)
			}

			if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
				t.Fatal("guard changed source")
			}

			if scenario == "nonempty" {
				data, err := os.ReadFile(filepath.Join(opts.OutDir, "keep"))
				if err != nil || string(data) != "keep" {
					t.Fatal("guard replaced output")
				}
			}
		})
	}
}

func TestRekey_ForceReplacesAndRestarts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("atomic directory exchange unsupported; tested by TestRekey_ExistingOutputUnsupported")
	}
	opts, dir := rekeyFixture(t)
	before := snapshotRekeyDir(t, dir)

	opts.OutDir = t.TempDir()
	if err := os.Mkdir(filepath.Join(opts.OutDir, "stale"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(opts.OutDir, "stale", "file"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts.Force = true

	res, err := Rekey(opts)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(res.Replaced, []string{"stale"}) {
		t.Fatal("replacement not reported")
	}

	if _, statErr := os.Stat(filepath.Join(opts.OutDir, "stale")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("stale output merged")
	}

	res, err = Rekey(opts)
	if err != nil || res.Items != 1 || len(res.Replaced) != 1 {
		t.Fatalf("restart failed: %v", err)
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("forced restart modified live directory")
	}
}

func TestRekey_EmptyClaimsNoChange(t *testing.T) {
	opts, dir := rekeyFixture(t)
	if err := openFileRing(t, dir, "old").Remove("credential/one"); err != nil {
		t.Fatal(err)
	}
	before := snapshotRekeyDir(t, dir)

	res, err := Rekey(opts)
	if err != nil || res.Items != 0 || res.PasswordChanged || res.BackupDir != "" {
		t.Fatalf("empty claimed change: %+v %v", res, err)
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("empty source modified")
	}

	entries, err := os.ReadDir(opts.OutDir)
	if err != nil || len(entries) != 0 {
		t.Fatal("empty output contains a verifier or credential")
	}
	// With no claimed rotation there is no stale-password guard to bypass by
	// removing credentials. Both Set and Remove on the live store still work.
	ring := openFileRing(t, dir, "old")
	if err := ring.Set(keyringItem("later", []byte("value"))); err != nil {
		t.Fatal(err)
	}

	if err := ring.Remove("later"); err != nil {
		t.Fatal(err)
	}
}

func TestRekey_Unreadable(t *testing.T) {
	opts, dir := rekeyFixture(t)
	if err := openFileRing(t, dir, "other").Set(keyringItem("unreadable", []byte("private-other"))); err != nil {
		t.Fatal(err)
	}
	before := snapshotRekeyDir(t, dir)
	opts.OutDir = t.TempDir()

	opts.Force = true
	if err := os.WriteFile(filepath.Join(opts.OutDir, "keep"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputBefore := snapshotRekeyDir(t, opts.OutDir)

	_, err := Rekey(opts)
	if !errors.Is(err, errRekeyUnreadableItems) {
		t.Fatalf("want unreadable error, got %v", err)
	}

	if !reflect.DeepEqual(outputBefore, snapshotRekeyDir(t, opts.OutDir)) {
		t.Fatal("unreadable input replaced output")
	}
	opts.SkipUnreadable = true

	res, err := Rekey(opts)
	if err != nil || res.Items != 1 || !reflect.DeepEqual(res.Unreadable, []string{"unreadable"}) {
		t.Fatalf("skip: %+v %v", res, err)
	}

	if _, err := openFileRing(t, opts.OutDir, "new").Get("unreadable"); !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatal("skipped credential present in output")
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("unreadable source changed")
	}
}

var errRekeyTestWrite = errors.New("injected output write failure")

type rekeyFaultRing struct {
	keyring.Keyring
	failSet, corruptGet, corruptMetadata, extraKey, failKeys bool
	targetKey                                                string
}

func (r *rekeyFaultRing) Set(item keyring.Item) error {
	if r.failSet && (r.targetKey == "" || item.Key == r.targetKey) {
		return errRekeyTestWrite
	}

	if err := r.Keyring.Set(item); err != nil {
		return fmt.Errorf("fault ring set: %w", err)
	}

	return nil
}

func (r *rekeyFaultRing) Get(key string) (keyring.Item, error) {
	item, err := r.Keyring.Get(key)
	if r.corruptGet && (r.targetKey == "" || key == r.targetKey) {
		item.Data = []byte("mismatch")
	}

	if r.corruptMetadata && (r.targetKey == "" || key == r.targetKey) {
		item.Label = "changed metadata"
	}

	if err != nil {
		return item, fmt.Errorf("fault ring get: %w", err)
	}

	return item, nil
}

func (r *rekeyFaultRing) Keys() ([]string, error) {
	if r.failKeys {
		return nil, errRekeyTestWrite
	}

	keys, err := r.Keyring.Keys()
	if r.extraKey {
		keys = append(keys, "unexpected")
	}

	if err != nil {
		return keys, fmt.Errorf("fault ring keys: %w", err)
	}

	return keys, nil
}

func TestRekey_VerifiesIndependentOutput(t *testing.T) {
	for _, scenario := range []string{"write", "mismatch", "metadata", "missing", "extra", "open", "list", "wrong-password"} {
		t.Run(scenario, func(t *testing.T) {
			opts, dir := rekeyFixture(t)
			// The first item is sound. Faults in a later item must still fail,
			// otherwise checking just one credential could pass this test.
			if err := openFileRing(t, dir, opts.OldPassword).Set(keyringItem("credential/two", []byte("second-private-payload"))); err != nil {
				t.Fatal(err)
			}
			before := snapshotRekeyDir(t, dir)
			opens := 0
			opts.openOutput = func(dir, password string) (keyring.Keyring, error) {
				opens++

				if password != opts.NewPassword {
					t.Fatal("output not opened with the new password")
				}

				if opens == 2 && scenario == "open" {
					return nil, errRekeyTestWrite
				}

				if opens == 2 && scenario == "wrong-password" {
					password = opts.OldPassword
				}

				ring := openFileRing(t, dir, password)
				if opens == 1 && scenario == "write" {
					return &rekeyFaultRing{Keyring: ring, failSet: true, targetKey: "credential/two"}, nil
				}

				if opens == 2 && scenario == "missing" {
					if err := ring.Remove("credential/two"); err != nil {
						t.Fatal(err)
					}
				}

				return &rekeyFaultRing{
					Keyring:         ring,
					corruptGet:      opens == 2 && scenario == "mismatch",
					corruptMetadata: opens == 2 && scenario == "metadata",
					extraKey:        opens == 2 && scenario == "extra",
					failKeys:        opens == 2 && scenario == "list",
					targetKey:       "credential/two",
				}, nil
			}

			res, err := Rekey(opts)
			if err == nil || res.Items != 0 || res.PasswordChanged || res.MoveCommand != "" {
				t.Fatalf("failed output reported success: %+v %v", res, err)
			}

			if scenario != "write" && opens != 2 {
				t.Fatal("verification did not independently reopen output")
			}

			want := errRekeyContentMismatch
			if scenario == "write" || scenario == "open" || scenario == "list" {
				want = errRekeyTestWrite
			}

			if !errors.Is(err, want) {
				t.Fatalf("want %v, got %v", want, err)
			}

			if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
				t.Fatal("failed output modified live directory")
			}

			for _, secret := range []string{"private-payload", "second-private-payload", "changed metadata"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("verification failure disclosed item content")
				}
			}
		})
	}
}

func TestRekey_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows mode bits do not represent Unix directory permissions; ACL equivalence is not exercised")
	}

	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			opts, dir := rekeyFixture(t)

			mode := os.FileMode(0o750) | os.ModeSetgid | os.ModeSticky
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			// Existing empty output must also receive the source's permissions.
			if existing {
				opts.OutDir = t.TempDir()
				if err := os.Chmod(opts.OutDir, 0o700); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := Rekey(opts); err != nil {
				t.Fatal(err)
			}

			info, err := os.Stat(opts.OutDir)
			if err != nil || info.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != mode {
				t.Fatalf("permissions differ: %v", err)
			}
		})
	}
}

func TestRekey_SymlinkOverlap(t *testing.T) {
	opts, dir := rekeyFixture(t)

	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink creation requires Windows privilege/developer mode: %v", err)
		}

		t.Fatal(err)
	}

	// A final symlink is refused even when it points away from the live keyring,
	// so --force can never remove or write a directory reached through a link.
	// Without this case the test passes on the overlap guard alone and the
	// symlink branch could be deleted unnoticed.
	benign := filepath.Join(t.TempDir(), "benign-alias")
	if err := os.Symlink(t.TempDir(), benign); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	for _, output := range []string{link, filepath.Join(link, "child", "missing"), benign} {
		opts.OutDir = output

		opts.Force = true
		if _, err := Rekey(opts); !errors.Is(err, errRekeyUnsafeOutput) {
			t.Fatalf("accepted symlink output %s: %v", output, err)
		}
	}
}

func TestRekey_PathIdentity(t *testing.T) {
	_, dir := rekeyFixture(t)

	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("directory aliases require symlink privilege/developer mode: %v", err)
		}

		t.Fatal(err)
	}
	// Exercise the identity comparison on every OS, even on a case-sensitive
	// filesystem. Neither spelling is lexically contained in the other.
	if !rekeyPathContains(dir, filepath.Join(link, "not-created")) || !rekeyPathContains(link, dir) {
		t.Fatal("directory identities were not compared")
	}
}

func TestRekey_CaseAliasOverlap(t *testing.T) {
	opts, dir := rekeyFixture(t)
	alias := filepath.Join(filepath.Dir(dir), strings.ToUpper(filepath.Base(dir)))

	info, err := os.Stat(alias)
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("temporary filesystem is case-sensitive; case aliases cannot be exercised")
	}

	if err != nil {
		t.Fatal(err)
	}

	original, err := os.Stat(dir)
	if err != nil || !os.SameFile(original, info) {
		t.Fatalf("case alias fixture is not the live directory: %v", err)
	}

	for _, output := range []string{alias, filepath.Join(alias, "new", "output")} {
		opts.OutDir, opts.Force = output, true
		if _, err := Rekey(opts); !errors.Is(err, errRekeyUnsafeOutput) {
			t.Fatalf("accepted case alias: %v", err)
		}
	}
}

func TestRekey_ReadFailures(t *testing.T) {
	for _, scenario := range []string{"missing", "file", "output-file", "nonregular"} {
		t.Run(scenario, func(t *testing.T) {
			opts, dir := rekeyFixture(t)

			switch scenario {
			case "missing":
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
			case "file":
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(dir, []byte("file"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "output-file":
				if err := os.WriteFile(opts.OutDir, []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "nonregular":
				if err := os.Symlink(filepath.Join(dir, fileSafeKey("credential/one")), filepath.Join(dir, "link")); err != nil {
					if runtime.GOOS == "windows" {
						t.Skipf("symlink privilege unavailable: %v", err)
					}

					t.Fatal(err)
				}
			}

			if _, err := Rekey(opts); err == nil {
				t.Fatal("filesystem failure reported success")
			}

			if scenario == "missing" {
				if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("source directory was created")
				}
			}
		})
	}
}

func TestRekey_LegacyAndConflicts(t *testing.T) {
	opts, dir := rekeyFixture(t)

	raw, err := keyring.Open(keyring.Config{AllowedBackends: []keyring.BackendType{keyring.FileBackend}, FileDir: dir, FilePasswordFunc: keyring.FixedStringPrompt("old")})
	if err != nil {
		t.Fatal(err)
	}
	// Raw filename is portable; slash and colon keys are tested encoded above.
	if setErr := raw.Set(keyringItem("legacy", []byte("legacy-data"))); setErr != nil {
		t.Fatal(setErr)
	}
	before := snapshotRekeyDir(t, dir)

	res, err := Rekey(opts)
	if err != nil || res.Items != 2 {
		t.Fatalf("legacy rekey: %v", err)
	}

	item, err := openFileRing(t, opts.OutDir, "new").Get("legacy")
	if err != nil || string(item.Data) != "legacy-data" {
		t.Fatal("legacy output missing")
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("legacy source modified")
	}

	if err := openFileRing(t, dir, "old").Set(keyringItem("legacy", []byte("different"))); err != nil {
		t.Fatal(err)
	}

	opts.Force = true
	if _, err := Rekey(opts); !errors.Is(err, errRekeyContentMismatch) {
		t.Fatalf("conflicting copies silently lost: %v", err)
	}
}

func TestRekey_LiveCommandsRemainUsable(t *testing.T) {
	opts, dir := rekeyFixture(t)
	// Historical markers and verifiers must have no influence on live writes.
	if err := os.WriteFile(dir+".rekey-in-progress", []byte("obsolete"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, fileSafeKey(keyringVerifierKey)), []byte("obsolete"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Rekey(opts); err != nil {
		t.Fatal(err)
	}

	ring := openFileRing(t, dir, "old")
	if _, err := ring.Get("credential/one"); err != nil {
		t.Fatalf("live credential cannot be read after rekey: %v", err)
	}

	if err := ring.Set(keyringItem("later", []byte("still-old"))); err != nil {
		t.Fatal(err)
	}

	if err := ring.Remove("credential/one"); err != nil {
		t.Fatal(err)
	}

	item, err := ring.Get("later")
	if err != nil || string(item.Data) != "still-old" {
		t.Fatal("live writes blocked or lost")
	}
}

func TestRekey_MoveCommand(t *testing.T) {
	for _, location := range []string{"separate", "old", "inside-old"} {
		t.Run(location, func(t *testing.T) {
			opts, dir := rekeyFixture(t)
			opts.OutDir = filepath.Join(t.TempDir(), "output ' $literal")

			switch location {
			case "old":
				opts.OutDir = dir + ".old"
			case "inside-old":
				opts.OutDir = filepath.Join(dir+".old", "output")
			}

			res, err := Rekey(opts)
			if err != nil {
				t.Fatal(err)
			}

			if runtime.GOOS != "windows" && !strings.Contains(res.MoveCommand, "mv --") {
				t.Fatal("missing exact move command")
			}
			// Execute the printed command against temp storage, then prove both copies.
			if output, runErr := rekeyMoveProcess(t, res.MoveCommand).CombinedOutput(); runErr != nil {
				t.Fatalf("move: %v %s", runErr, output)
			}

			if _, err := openFileRing(t, dir, "new").Get("credential/one"); err != nil {
				t.Fatal(err)
			}

			if _, err := openFileRing(t, res.RetainedDir, "old").Get("credential/one"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func rekeyMoveProcess(t *testing.T, command string) *exec.Cmd {
	t.Helper()

	if runtime.GOOS == "windows" {
		for _, shell := range []string{"pwsh", "powershell"} {
			if path, err := exec.LookPath(shell); err == nil {
				return exec.CommandContext(context.Background(), path, "-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference = 'Stop'; "+command)
			}
		}

		t.Skip("PowerShell is unavailable; cannot execute the Windows installation command")
	}

	return exec.CommandContext(context.Background(), "sh", "-c", command)
}

func TestRekey_MoveCommandProtectsOld(t *testing.T) {
	for _, kind := range []string{"directory", "dangling-symlink"} {
		t.Run(kind, func(t *testing.T) {
			opts, dir := rekeyFixture(t)

			res, err := Rekey(opts)
			if err != nil {
				t.Fatal(err)
			}

			old := dir + ".old"
			if kind == "directory" {
				if err := os.Mkdir(old, 0o700); err != nil {
					t.Fatal(err)
				}

				if err := os.WriteFile(filepath.Join(old, "keep"), []byte("keep"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(filepath.Join(t.TempDir(), "missing"), old); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("dangling symlink creation requires privilege/developer mode: %v", err)
				}

				t.Fatal(err)
			}
			// Keep both source operands present so failure must be the .old
			// guard, and snapshot all three paths to catch partial moves too.
			liveBefore := snapshotRekeyDir(t, dir)
			outBefore := snapshotRekeyDir(t, opts.OutDir)

			oldBefore := snapshotRekeyDir(t, old)
			// A dangling destination can make mv fail independently of our
			// guard. Record attempted moves so that removing the guard cannot
			// pass this test merely because the platform's mv also refuses it.
			probe := filepath.Join(t.TempDir(), "attempted-move")

			command := "rekey_probe_move() { printf 'called\\n' >> " + shellQuote(probe) + "; mv \"$@\"; }; " + strings.ReplaceAll(res.MoveCommand, "mv --", "rekey_probe_move --")
			if runtime.GOOS == "windows" {
				quote := "'" + strings.ReplaceAll(probe, "'", "''") + "'"
				command = "function Rekey-ProbeMove { [CmdletBinding()] param([string]$LiteralPath, [string]$Destination) Add-Content -LiteralPath " + quote + " -Value 'called'; Microsoft.PowerShell.Management\\Move-Item @PSBoundParameters }; " + strings.ReplaceAll(res.MoveCommand, "Move-Item", "Rekey-ProbeMove")
			}

			if output, err := rekeyMoveProcess(t, command).CombinedOutput(); err == nil {
				t.Fatalf("installation accepted existing .old: %s", output)
			}

			if _, err := os.Stat(probe); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("installation attempted a move despite existing .old")
			}

			if !reflect.DeepEqual(liveBefore, snapshotRekeyDir(t, dir)) ||
				!reflect.DeepEqual(outBefore, snapshotRekeyDir(t, opts.OutDir)) ||
				!reflect.DeepEqual(oldBefore, snapshotRekeyDir(t, old)) {
				t.Fatal("refused installation changed directories")
			}
		})
	}
}

func TestRekey_FailureReportsReplacement(t *testing.T) {
	opts, dir := rekeyFixture(t)

	opts.OutDir, opts.Force = t.TempDir(), true
	if err := os.WriteFile(filepath.Join(opts.OutDir, "stale"), []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts.openOutput = func(string, string) (keyring.Keyring, error) { return nil, errRekeyTestWrite }
	before := snapshotRekeyDir(t, dir)

	res, err := Rekey(opts)
	if !errors.Is(err, errRekeyTestWrite) || len(res.Replaced) != 0 || res.MoveCommand != "" || res.Items != 0 {
		t.Fatalf("failed run lost replacement information: %+v %v", res, err)
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("failed replacement modified live directory")
	}
}

func TestRekey_BookkeepingIsReadOnly(t *testing.T) {
	opts, dir := rekeyFixture(t)
	for _, name := range []string{keyringLockFilename, keyringVerifierKey, fileSafeKey(keyringVerifierKey), ".DS_Store"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("not-a-credential"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshotRekeyDir(t, dir)

	res, err := Rekey(opts)
	if err != nil || res.Items != 1 || len(res.Unreadable) != 0 {
		t.Fatalf("bookkeeping treated as credentials: %+v %v", res, err)
	}

	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("bookkeeping changed")
	}
}

func TestCheckRekeyBackend(t *testing.T) {
	for _, tc := range []struct {
		backend, goos, dbus string
		allowed             bool
	}{
		{"file", "windows", "", true},
		{"file", "darwin", "", true},
		{"auto", "linux", "", true},
		{"auto", "linux", "session", false},
		{"auto", "darwin", "", false},
		{"auto", "windows", "", false},
		{"keychain", "linux", "", false},
	} {
		err := CheckRekeyBackend(tc.backend, tc.goos, tc.dbus)
		if (err == nil) != tc.allowed {
			t.Fatalf("backend %s/%s/%s: %v", tc.backend, tc.goos, tc.dbus, err)
		}
	}
}
