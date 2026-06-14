//nolint:err113,wrapcheck,wsl_v5 // Git helper returns command-context errors.
package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type noInputContextKey struct{}

var gitURLPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*://[^\s<>"']+`)

func WithNoInput(ctx context.Context) context.Context {
	return context.WithValue(ctx, noInputContextKey{}, true)
}

func ensureRepo(ctx context.Context, cfg Config) error {
	if strings.TrimSpace(cfg.Repo) == "" {
		return fmt.Errorf("backup repo path is required")
	}
	if _, err := os.Stat(filepath.Join(cfg.Repo, ".git")); err == nil {
		return pullRepo(ctx, cfg.Repo)
	}
	if strings.TrimSpace(cfg.Remote) != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Repo), 0o700); err != nil {
			return err
		}
		if err := git(ctx, "", "clone", cfg.Remote, cfg.Repo); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(cfg.Repo, 0o700); err != nil {
		return err
	}
	if err := git(ctx, cfg.Repo, "init"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Remote) != "" {
		if err := git(ctx, cfg.Repo, "remote", "add", "origin", cfg.Remote); err != nil {
			return err
		}
	}
	return nil
}

func prepareReadRepo(ctx context.Context, cfg Config, skipPull bool) error {
	if strings.TrimSpace(cfg.Repo) == "" {
		return fmt.Errorf("backup repo path is required")
	}
	if skipPull {
		return nil
	}

	gitPath := filepath.Join(cfg.Repo, ".git")
	if _, err := os.Stat(gitPath); err == nil {
		return pullRepo(ctx, cfg.Repo)
	} else if !os.IsNotExist(err) {
		return err
	}

	if strings.TrimSpace(cfg.Remote) == "" {
		return fmt.Errorf("backup repo is not initialized at %s; run 'gog backup init' or provide --remote", cfg.Repo)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Repo), 0o700); err != nil {
		return err
	}
	return cloneReadRepo(ctx, cfg.Remote, cfg.Repo)
}

func cloneReadRepo(ctx context.Context, remote, repo string) error {
	info, statErr := os.Stat(repo)
	if statErr == nil {
		return cloneReadRepoIntoExisting(ctx, remote, repo, info)
	}
	if !os.IsNotExist(statErr) {
		return statErr
	}

	parent := filepath.Dir(repo)
	tempRepo, err := os.MkdirTemp(parent, "."+filepath.Base(repo)+".clone-*")
	if err != nil {
		return err
	}
	defer func() {
		if tempRepo != "" {
			_ = os.RemoveAll(tempRepo)
		}
	}()

	if err := git(ctx, "", "clone", remote, tempRepo); err != nil {
		return err
	}
	if err := os.Rename(tempRepo, repo); err != nil {
		return fmt.Errorf("install cloned backup repo: %w", err)
	}
	tempRepo = ""
	return nil
}

func cloneReadRepoIntoExisting(ctx context.Context, remote, repo string, info os.FileInfo) error {
	if !info.IsDir() {
		return fmt.Errorf("backup repo path exists but is not a directory: %s", repo)
	}
	entries, err := os.ReadDir(repo)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("backup repo path exists but is not a Git repository or empty directory: %s", repo)
	}

	if err := git(ctx, "", "clone", remote, repo); err != nil {
		if cleanupErr := removeDirectoryContents(repo); cleanupErr != nil {
			return errors.Join(err, fmt.Errorf("clean failed clone: %w", cleanupErr))
		}
		return err
	}
	return nil
}

func removeDirectoryContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func pullRepo(ctx context.Context, repo string) error {
	pullErr := git(ctx, repo, "pull", "--rebase")
	if pullErr == nil {
		return nil
	}
	hasHead := git(ctx, repo, "rev-parse", "--verify", "HEAD") == nil
	if !hasHead {
		return nil
	}
	if strings.Contains(pullErr.Error(), "no tracking information") ||
		strings.Contains(pullErr.Error(), "No remote repository specified") ||
		strings.Contains(pullErr.Error(), "no such ref was fetched") {
		return nil
	}
	return pullErr
}

func commitAndPush(ctx context.Context, cfg Config, message string, push bool) (bool, error) {
	changed, _, err := commitChanges(ctx, cfg, message)
	if err != nil || !changed || !push {
		return changed, err
	}
	if err := git(ctx, cfg.Repo, "push", "-u", "origin", "HEAD"); err != nil {
		return true, err
	}
	return true, nil
}

func commitChanges(ctx context.Context, cfg Config, message string) (bool, string, error) {
	if err := removeTempShardFiles(cfg.Repo); err != nil {
		return false, "", err
	}
	if err := git(ctx, cfg.Repo, "add", "."); err != nil {
		return false, "", err
	}
	if err := git(ctx, cfg.Repo, "diff", "--cached", "--quiet"); err == nil {
		return false, "", nil
	}
	if err := git(ctx, cfg.Repo, "-c", "commit.gpgsign=false", "commit", "-m", message); err != nil {
		return false, "", err
	}
	sha, err := gitOutput(ctx, cfg.Repo, "rev-parse", "HEAD")
	if err != nil {
		return true, "", err
	}
	return true, strings.TrimSpace(sha), nil
}

func pushCommit(ctx context.Context, cfg Config, sha string) error {
	branch, err := gitOutput(ctx, cfg.Repo, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || strings.TrimSpace(branch) == "" {
		branch = "main"
	}
	refspec := strings.TrimSpace(sha) + ":refs/heads/" + strings.TrimSpace(branch)
	return git(ctx, cfg.Repo, "push", "-u", "origin", refspec)
}

func git(ctx context.Context, dir string, args ...string) error {
	cmd := gitCommand(ctx, dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return gitError(args, err, stderr.String())
	}
	return nil
}

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := gitCommand(ctx, dir, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", gitError(args, err, stderr.String())
	}
	return stdout.String(), nil
}

func gitError(args []string, err error, stderr string) error {
	safeArgs := make([]string, len(args))
	safeStderr := redactGitURLsInText(strings.TrimSpace(stderr))
	for i, arg := range args {
		safeArgs[i] = redactGitURL(arg)
		if safeArgs[i] != arg {
			safeStderr = strings.ReplaceAll(safeStderr, arg, safeArgs[i])
		}
	}
	if safeStderr != "" {
		return fmt.Errorf("git %s: %w: %s", strings.Join(safeArgs, " "), err, safeStderr)
	}
	return fmt.Errorf("git %s: %w", strings.Join(safeArgs, " "), err)
}

func redactGitURLsInText(value string) string {
	return gitURLPattern.ReplaceAllStringFunc(value, redactGitURL)
}

func redactGitURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}

	redacted := false
	if parsed.User != nil {
		parsed.User = url.User("redacted")
		redacted = true
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key, values := range query {
			for i := range values {
				values[i] = "redacted"
			}
			query[key] = values
		}
		parsed.RawQuery = query.Encode()
		redacted = true
	}
	if parsed.Fragment != "" {
		parsed.Fragment = "redacted"
		redacted = true
	}
	if !redacted {
		return value
	}
	return parsed.String()
}

func gitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- callers pass fixed git subcommands plus configured repo paths.
	cmd.Dir = dir
	cmd.Env = gitEnvironment(ctx)
	return cmd
}

func gitEnvironment(ctx context.Context) []string {
	values := map[string]string{
		"GIT_AUTHOR_NAME":     "gog",
		"GIT_AUTHOR_EMAIL":    "gog@example.invalid",
		"GIT_COMMITTER_NAME":  "gog",
		"GIT_COMMITTER_EMAIL": "gog@example.invalid",
	}
	if noInput, _ := ctx.Value(noInputContextKey{}).(bool); noInput {
		values["GIT_TERMINAL_PROMPT"] = "0"
		values["GCM_INTERACTIVE"] = "Never"
		values["GIT_ASKPASS"] = ""
		values["SSH_ASKPASS"] = ""
		sshCommand := strings.TrimSpace(os.Getenv("GIT_SSH_COMMAND"))
		if sshCommand == "" {
			sshCommand = "ssh"
		}
		values["GIT_SSH_COMMAND"] = sshCommand + " -o BatchMode=yes"
	}
	return replaceEnvironment(os.Environ(), values)
}

func replaceEnvironment(base []string, values map[string]string) []string {
	env := make([]string, 0, len(base)+len(values))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if _, replace := values[key]; ok && replace {
			continue
		}
		env = append(env, entry)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}
