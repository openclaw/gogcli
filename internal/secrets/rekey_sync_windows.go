package secrets

import "golang.org/x/sys/windows"

// Windows has no directory fsync. Publication uses a write-through rename;
// ciphertext files are individually flushed before it.
func syncRekeyDir(string) error { return nil }

func renameRekeyDir(from, to string) error {
	source, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(source, target, windows.MOVEFILE_WRITE_THROUGH)
}
