//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveScript(t *testing.T) {
	if firstNonEmpty(os.Getenv("RATA_LIVE"), os.Getenv("GOG_LIVE")) == "" {
		t.Skip("set RATA_LIVE=1 to run live tests")
	}

	root := findRepoRoot(t)
	script := filepath.Join(root, "scripts", "live-test.sh")

	args := []string{}
	if firstNonEmpty(os.Getenv("RATA_LIVE_FAST"), os.Getenv("GOG_LIVE_FAST")) != "" {
		args = append(args, "--fast")
	}
	if firstNonEmpty(os.Getenv("RATA_LIVE_STRICT"), os.Getenv("GOG_LIVE_STRICT")) != "" {
		args = append(args, "--strict")
	}
	if v := firstNonEmpty(os.Getenv("RATA_LIVE_ACCOUNT"), os.Getenv("GOG_LIVE_ACCOUNT"), os.Getenv("RATA_IT_ACCOUNT"), os.Getenv("GOG_IT_ACCOUNT")); v != "" {
		args = append(args, "--account", v)
	}
	if v := firstNonEmpty(os.Getenv("RATA_LIVE_SKIP"), os.Getenv("GOG_LIVE_SKIP")); v != "" {
		args = append(args, "--skip", v)
	}
	if v := firstNonEmpty(os.Getenv("RATA_LIVE_AUTH"), os.Getenv("GOG_LIVE_AUTH")); v != "" {
		args = append(args, "--auth", v)
	}
	if firstNonEmpty(os.Getenv("RATA_LIVE_ALLOW_NONTEST"), os.Getenv("GOG_LIVE_ALLOW_NONTEST")) != "" {
		args = append(args, "--allow-nontest")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, script, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		t.Fatalf("live test script failed: %v", err)
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	t.Fatalf("repo root not found from %s", cwd)
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
