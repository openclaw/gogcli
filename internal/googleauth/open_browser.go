package googleauth

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"time"
)

var startCommand = func(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// chromeCleanup tracks the headless Chrome process for cleanup
var (
	chromeMu   sync.Mutex
	chromePid  int
	debugPort  = "9222"
	debugPortRe = regexp.MustCompile(`--remote-debugging-port=(\d+)`)
)

func openBrowser(u string) error {
	// In headless environments (no display), use Chrome directly for cleanup control
	if isHeadless() {
		return startHeadlessChrome(u)
	}

	name, args := openBrowserCommand(u, runtime.GOOS)
	return startCommand(name, args...)
}

func isHeadless() bool {
	// Check common headless indicators
	if runtime.GOOS == "linux" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return true
		}
	}
	return false
}

func startHeadlessChrome(url string) error {
	// Kill any existing Chrome on our debug port to avoid conflicts
	cleanupChrome()

	// Find Chrome binary
	chromePath := findChromePath()
	if chromePath == "" {
		// Fall back to xdg-open if Chrome not found
		name, args := openBrowserCommand(url, runtime.GOOS)
		return startCommand(name, args...)
	}

	// Start Chrome in headless mode with debug port
	cmd := exec.Command(chromePath,
		"--headless",
		"--no-first-run",
		"--no-sandbox",
		"--disable-dev-shm-usage",
		"--remote-debugging-port="+debugPort,
		url,
	)

	if err := cmd.Start(); err != nil {
		return err
	}

	chromeMu.Lock()
	chromePid = cmd.Process.Pid
	chromeMu.Unlock()

	return nil
}

func findChromePath() string {
	// Common Chrome binary locations
	paths := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"/usr/bin/google-chrome",
		"/usr/bin/chromium",
		"/usr/bin/chromium-browser",
	}

	for _, p := range paths {
		cmd := exec.Command("which", p)
		if err := cmd.Run(); err == nil {
			return p
		}
	}

	return ""
}

// CleanupChrome kills any headless Chrome process we spawned
func CleanupChrome() {
	cleanupChrome()
}

func cleanupChrome() {
	chromeMu.Lock()
	pid := chromePid
	chromePid = 0
	chromeMu.Unlock()

	if pid > 0 {
		// First try SIGTERM for graceful shutdown
		exec.Command("kill", "-TERM", "-"+strconv.Itoa(pid)).Run()
		time.Sleep(500 * time.Millisecond)

		// Then force kill if still running
		exec.Command("kill", "-9", "-"+strconv.Itoa(pid)).Run()
	}

	// Also clean up any stale Chrome on our debug port
	exec.Command("pkill", "-f", "chrome.*--remote-debugging-port="+debugPort).Run()
}
