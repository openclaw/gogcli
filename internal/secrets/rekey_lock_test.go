package secrets

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Rekey does not acquire the live lock or block other commands. Exercise the
// platform's actual locking implementation on Linux, macOS and Windows.
func TestRekey_DoesNotTakeLiveLock(t *testing.T) {
	opts, dir := rekeyFixture(t)

	holder, err := os.OpenFile(filepath.Join(dir, keyringLockFilename), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()

	before := snapshotRekeyDir(t, dir)

	func() {
		if err := lockKeyringFile(holder, true); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := unlockKeyringFile(holder); err != nil {
				t.Error(err)
			}
		}()

		if _, err := Rekey(opts); err != nil {
			t.Fatal(err)
		}
	}()
	// Windows locks also prevent reading the locked byte range. Snapshot only
	// after unlocking, so the test cannot fail on its own snapshot read.
	if !reflect.DeepEqual(before, snapshotRekeyDir(t, dir)) {
		t.Fatal("locked source modified")
	}
}
