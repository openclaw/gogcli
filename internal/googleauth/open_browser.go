package googleauth

import (
	"os/exec"
	"runtime"
)

var startCommand = func(name string, args ...string) error {
	return exec.Command(name, args...).Start() //nolint:gosec // command/args come from fixed OS mapping and caller URL
}

func openBrowser(u string) error {
	name, args := openBrowserCommand(u, runtime.GOOS)
	return startCommand(name, args...)
}

func openBrowserCommand(u string, goos string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{u}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", u}
	default:
		return "xdg-open", []string{u}
	}
}
