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

	if _, statErr := os.Stat(filepath.Join(repo, ".git")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("clone failure initialized repo: %v", statErr)
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
