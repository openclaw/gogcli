package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadOperationsDoNotInitializeMissingRepo(t *testing.T) {
	operations := []struct {
		name string
		run  func(context.Context, Options) error
	}{
		{
			name: "status",
			run: func(ctx context.Context, opts Options) error {
				_, _, err := Status(ctx, opts)
				return err
			},
		},
		{
			name: "verify",
			run: func(ctx context.Context, opts Options) error {
				_, err := Verify(ctx, opts)
				return err
			},
		},
		{
			name: "cat",
			run: func(ctx context.Context, opts Options) error {
				_, err := Cat(ctx, opts, "data/test.jsonl.gz.age")
				return err
			},
		},
		{
			name: "walk",
			run: func(ctx context.Context, opts Options) error {
				_, _, err := WalkSnapshot(ctx, opts, nil)
				return err
			},
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			dir := t.TempDir()
			repo := filepath.Join(dir, "repo")
			configPath := filepath.Join(dir, "backup.json")
			saveTestConfig(t, configPath, Config{Repo: repo})

			err := operation.run(t.Context(), testOptions(t, Options{ConfigPath: configPath}))
			if err == nil || !strings.Contains(err.Error(), "backup repo is not initialized") {
				t.Fatalf("error = %v, want not-initialized guidance", err)
			}

			if _, statErr := os.Stat(repo); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("read operation created repo: %v", statErr)
			}
		})
	}
}

func TestReadRepoCloneFailureDoesNotInitializeRepo(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	configPath := filepath.Join(dir, "backup.json")
	remote := filepath.Join(dir, "missing-remote.git")
	saveTestConfig(t, configPath, Config{Repo: repo, Remote: remote})

	_, _, err := Status(t.Context(), testOptions(t, Options{ConfigPath: configPath}))
	if err == nil || !strings.Contains(err.Error(), "git clone") {
		t.Fatalf("error = %v, want clone failure", err)
	}

	if _, statErr := os.Stat(repo); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("clone failure left repo behind: %v", statErr)
	}

	matches, globErr := filepath.Glob(filepath.Join(dir, ".repo.clone-*"))
	if globErr != nil {
		t.Fatalf("Glob: %v", globErr)
	}

	if len(matches) != 0 {
		t.Fatalf("clone failure left temporary repos: %v", matches)
	}
}

func TestNoInputGitEnvironmentDisablesPrompts(t *testing.T) {
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GCM_INTERACTIVE", "Always")
	t.Setenv("GIT_ASKPASS", "askpass")
	t.Setenv("SSH_ASKPASS", "ssh-askpass")
	t.Setenv("GIT_SSH_COMMAND", "ssh -i test-key")

	env := gitEnvironment(WithNoInput(t.Context()))
	values := make(map[string][]string)

	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = append(values[key], value)
		}
	}

	for key, want := range map[string]string{
		"GIT_TERMINAL_PROMPT": "0",
		"GCM_INTERACTIVE":     "Never",
		"GIT_ASKPASS":         "",
		"SSH_ASKPASS":         "",
		"GIT_SSH_COMMAND":     "ssh -i test-key -o BatchMode=yes",
	} {
		if got := values[key]; len(got) != 1 || got[0] != want {
			t.Fatalf("%s = %q, want [%q]", key, got, want)
		}
	}
}

func TestReadOperationsSkipPullWithoutCreatingRepo(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	configPath := filepath.Join(dir, "backup.json")
	saveTestConfig(t, configPath, Config{Repo: repo})

	_, _, err := Status(t.Context(), testOptions(t, Options{ConfigPath: configPath, SkipPull: true}))
	if err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("error = %v, want missing manifest", err)
	}

	if _, statErr := os.Stat(repo); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("--no-pull created repo: %v", statErr)
	}
}
