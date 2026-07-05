package secrets

import (
	"errors"
	"testing"
)

var errCodesignFailed = errors.New("codesign failed")

func TestResolveKeychainTrustApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		goos        string
		backend     string
		override    string
		output      string
		runnerErr   error
		want        bool
		wantForced  bool
		wantRuns    int
		wantApplies bool
	}{
		{name: "unsigned", goos: "darwin", backend: "keychain", output: "code object is not signed at all", runnerErr: errCodesignFailed, wantRuns: 1, wantApplies: true},
		{name: "ad-hoc", goos: "darwin", backend: "keychain", output: "Executable=/tmp/gog\nSignature=adhoc\nTeamIdentifier=not set", wantRuns: 1, wantApplies: true},
		{name: "developer id", goos: "darwin", backend: "keychain", output: "Executable=/tmp/gog\nIdentifier=org.openclaw.gog\nTeamIdentifier=Y5PE65HELJ", want: true, wantRuns: 1, wantApplies: true},
		{name: "team identifier not set", goos: "darwin", backend: "auto", output: "Signature=adhoc\nTeamIdentifier=not set", wantRuns: 1, wantApplies: true},
		{name: "runner error", goos: "darwin", backend: "auto", runnerErr: errCodesignFailed, wantRuns: 1, wantApplies: true},
		{name: "non-darwin", goos: "linux", backend: "keychain", override: "true", wantRuns: 0},
		{name: "file backend", goos: "darwin", backend: "file", override: "true", wantRuns: 0},
		{name: "forced true", goos: "darwin", backend: "keychain", override: "TrUe", want: true, wantForced: true, wantRuns: 0, wantApplies: true},
		{name: "forced one", goos: "darwin", backend: "keychain", override: "1", want: true, wantForced: true, wantRuns: 0, wantApplies: true},
		{name: "forced false", goos: "darwin", backend: "keychain", override: "FALSE", wantForced: true, wantRuns: 0, wantApplies: true},
		{name: "forced zero", goos: "darwin", backend: "keychain", override: "0", wantForced: true, wantRuns: 0, wantApplies: true},
		{name: "invalid uses auto", goos: "darwin", backend: "keychain", override: "sometimes", output: "TeamIdentifier=Y5PE65HELJ", want: true, wantRuns: 1, wantApplies: true},
		{name: "explicit auto", goos: "darwin", backend: "keychain", override: "AUTO", output: "TeamIdentifier=Y5PE65HELJ", want: true, wantRuns: 1, wantApplies: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runs := 0
			options := OpenOptions{
				GOOS:                     tt.goos,
				KeychainTrustApplication: tt.override,
				codesignRunner: func(path string) ([]byte, error) {
					runs++

					if path == "" {
						t.Fatal("codesign path is empty")
					}

					return []byte(tt.output), tt.runnerErr
				},
			}

			info := ResolveKeychainTrustApplication(options, KeyringBackendInfo{Value: tt.backend})
			if info.Enabled != tt.want || info.Forced != tt.wantForced || info.Applicable != tt.wantApplies {
				t.Fatalf("info = %#v, want enabled=%t forced=%t applicable=%t", info, tt.want, tt.wantForced, tt.wantApplies)
			}

			if runs != tt.wantRuns {
				t.Fatalf("codesign runs = %d, want %d", runs, tt.wantRuns)
			}
		})
	}
}

func TestCodesignOutputHasStableIdentityRejectsAdhocBeforeTeamIdentifier(t *testing.T) {
	t.Parallel()

	output := []byte("Signature=adhoc\nTeamIdentifier=Y5PE65HELJ\n")
	if codesignOutputHasStableIdentity(output) {
		t.Fatal("ad-hoc signature must not be trusted")
	}
}
