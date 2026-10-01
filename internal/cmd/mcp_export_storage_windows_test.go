package cmd

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestMCPExportStorageProtectedUserDACL(t *testing.T) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []bool{true, false} {
		path := filepath.Join(t.TempDir(), "private")
		if directory {
			err = os.Mkdir(path, 0o700)
		} else {
			err = os.WriteFile(path, nil, 0o600)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = makeMCPStoragePrivate(path, directory); err != nil {
			t.Fatal(err)
		}
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		control, _, err := sd.Control()
		if err != nil {
			t.Fatal(err)
		}
		if acl == nil || acl.AceCount != 1 || control&windows.SE_DACL_PROTECTED == 0 {
			t.Fatalf("DACL is not protected and exclusive to the current user: %s", sd.String())
		}
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, 0, &ace); err != nil {
			t.Fatal(err)
		}
		trustee := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || ace.Header.AceFlags != 0 || ace.Mask != fileAllAccess || !trustee.Equals(user.User.Sid) {
			t.Fatalf("DACL does not grant explicit full access exclusively to the current user: %s", sd.String())
		}
	}
}

func TestMCPExportStorageRejectsJunction(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	junction := filepath.Join(root, "junction")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(junction, 0o700); err != nil {
		t.Fatal(err)
	}
	name, err := windows.UTF16PtrFromString(junction)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	substitute := utf16.Encode([]rune(`\??\` + target))
	display := utf16.Encode([]rune(target))
	pathData := append(append(append(substitute, 0), display...), 0)
	buffer := make([]byte, 16+2*len(pathData))
	binary.LittleEndian.PutUint32(buffer, windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buffer[4:], uint16(len(buffer)-8))
	binary.LittleEndian.PutUint16(buffer[10:], uint16(2*len(substitute)))
	binary.LittleEndian.PutUint16(buffer[12:], uint16(2*(len(substitute)+1)))
	binary.LittleEndian.PutUint16(buffer[14:], uint16(2*len(display)))
	for i, value := range pathData {
		binary.LittleEndian.PutUint16(buffer[16+2*i:], value)
	}
	var returned uint32
	err = windows.DeviceIoControl(handle, windows.FSCTL_SET_REPARSE_POINT, &buffer[0], uint32(len(buffer)), nil, 0, &returned, nil)
	_ = windows.CloseHandle(handle)
	if err != nil {
		t.Fatal("create negative junction fixture", err)
	}
	if handle, err = openMCPPrivateStorageObject(junction, true); err == nil {
		_ = windows.CloseHandle(handle)
		t.Fatal("junction accepted as private storage")
	}
	if err = makeMCPStoragePrivate(junction, true); err == nil {
		t.Fatal("private DACL followed junction target")
	}
}
