package cmd

import (
	"strings"
	"testing"
)

func TestBuildLaunchdPlist(t *testing.T) {
	const execPath = "/usr/local/bin/gog"
	const intervalSecs = 300

	got := buildLaunchdPlist(execPath, intervalSecs)

	// Must contain the correct label.
	if !strings.Contains(got, launchdLabel) {
		t.Errorf("plist missing label %q:\n%s", launchdLabel, got)
	}

	// ProgramArguments must include the binary path.
	if !strings.Contains(got, execPath) {
		t.Errorf("plist missing execPath %q:\n%s", execPath, got)
	}

	// Must include the wake subcommand arguments.
	for _, arg := range []string{"gmail", "snooze", "wake", "--all"} {
		if !strings.Contains(got, "<string>"+arg+"</string>") {
			t.Errorf("plist missing ProgramArguments entry %q:\n%s", arg, got)
		}
	}

	// StartInterval value must appear.
	if !strings.Contains(got, "<integer>300</integer>") {
		t.Errorf("plist missing StartInterval 300:\n%s", got)
	}

	// Sanity: must be valid-ish XML.
	if !strings.HasPrefix(strings.TrimSpace(got), "<?xml") {
		t.Errorf("plist does not start with <?xml:\n%s", got)
	}
}

func TestBuildLaunchdPlist_CustomInterval(t *testing.T) {
	got := buildLaunchdPlist("/bin/gog", 60)
	if !strings.Contains(got, "<integer>60</integer>") {
		t.Errorf("plist missing StartInterval 60:\n%s", got)
	}
}

func TestBuildSystemdService(t *testing.T) {
	const execPath = "/home/user/.local/bin/gog"

	got := buildSystemdService(execPath)

	// ExecStart must contain the binary path.
	if !strings.Contains(got, "ExecStart="+execPath) {
		t.Errorf("service unit missing ExecStart with path %q:\n%s", execPath, got)
	}

	// Must include the wake subcommand.
	if !strings.Contains(got, "snooze wake") {
		t.Errorf("service unit missing 'snooze wake':\n%s", got)
	}

	// Must include --all flag.
	if !strings.Contains(got, "--all") {
		t.Errorf("service unit missing '--all':\n%s", got)
	}

	// Must have [Service] section.
	if !strings.Contains(got, "[Service]") {
		t.Errorf("service unit missing [Service] section:\n%s", got)
	}
}

func TestBuildSystemdTimer(t *testing.T) {
	const unitName = systemdUnit
	const intervalSecs = 300

	got := buildSystemdTimer(unitName, intervalSecs)

	// OnUnitActiveSec must reflect the interval.
	if !strings.Contains(got, "OnUnitActiveSec=300") {
		t.Errorf("timer unit missing OnUnitActiveSec=300:\n%s", got)
	}

	// Unit= must reference the service.
	if !strings.Contains(got, "Unit="+unitName+".service") {
		t.Errorf("timer unit missing Unit=%s.service:\n%s", unitName, got)
	}

	// Must have [Timer] section.
	if !strings.Contains(got, "[Timer]") {
		t.Errorf("timer unit missing [Timer] section:\n%s", got)
	}

	// Must have [Install] section for enabling.
	if !strings.Contains(got, "[Install]") {
		t.Errorf("timer unit missing [Install] section:\n%s", got)
	}
}

func TestBuildSystemdTimer_CustomInterval(t *testing.T) {
	got := buildSystemdTimer(systemdUnit, 120)
	if !strings.Contains(got, "OnUnitActiveSec=120") {
		t.Errorf("timer unit missing OnUnitActiveSec=120:\n%s", got)
	}
}
