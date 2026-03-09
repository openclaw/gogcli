package tracking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type DeployLogger interface {
	Printf(format string, args ...any)
}

type DeployOptions struct {
	WorkerDir    string
	WorkerName   string
	DatabaseName string
	TrackingKey  string
	AdminKey     string
}

var (
	errWranglerNotFound      = errors.New("wrangler not found in PATH")
	errWorkerConfigMissing   = errors.New("worker dir missing wrangler.toml")
	errParseDatabaseIDInfo   = errors.New("failed to parse database_id from wrangler d1 info output")
	errParseDatabaseIDCreate = errors.New("failed to parse database_id from wrangler d1 create output")
)

func DefaultWorkerName(account string) string {
	sanitized := SanitizeWorkerName(account)
	if sanitized == "" {
		return "gog-email-tracker"
	}

	return "gog-email-tracker-" + sanitized
}

func SanitizeWorkerName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}

	re := regexp.MustCompile(`[^a-z0-9-]+`)
	name = re.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}

	if len(name) > 63 {
		name = strings.Trim(name[:63], "-")
	}

	return name
}

func DeployWorker(ctx context.Context, logger DeployLogger, opts DeployOptions) (string, error) {
	if _, err := exec.LookPath("wrangler"); err != nil {
		return "", errWranglerNotFound
	}

	workerDir := filepath.Clean(opts.WorkerDir)
	if _, err := os.Stat(filepath.Join(workerDir, "wrangler.toml")); err != nil {
		return "", fmt.Errorf("%w: %s", errWorkerConfigMissing, workerDir)
	}

	if logger != nil {
		logger.Printf("deploy\tstarting (worker=%s, db=%s)", opts.WorkerName, opts.DatabaseName)
	}

	dbID, err := ensureD1Database(ctx, workerDir, opts.DatabaseName)
	if err != nil {
		return "", err
	}

	if runErr := runWranglerCommand(ctx, workerDir, nil, "d1", "execute", opts.DatabaseName, "--file", "schema.sql", "--remote"); runErr != nil {
		return "", runErr
	}

	if runErr := runWranglerCommand(ctx, workerDir, strings.NewReader(opts.TrackingKey+"\n"), "secret", "put", "TRACKING_KEY", "--name", opts.WorkerName); runErr != nil {
		return "", runErr
	}

	if runErr := runWranglerCommand(ctx, workerDir, strings.NewReader(opts.AdminKey+"\n"), "secret", "put", "ADMIN_KEY", "--name", opts.WorkerName); runErr != nil {
		return "", runErr
	}

	configPath, err := writeWranglerConfig(workerDir, opts.WorkerName, opts.DatabaseName, dbID)
	if err != nil {
		return "", err
	}
	defer os.Remove(configPath)

	if runErr := runWranglerCommand(ctx, workerDir, nil, "deploy", "--config", configPath, "--name", opts.WorkerName); runErr != nil {
		return "", runErr
	}

	if logger != nil {
		logger.Printf("deploy\tok")
	}

	return dbID, nil
}

func ensureD1Database(ctx context.Context, workerDir, dbName string) (string, error) {
	out, err := runWranglerCommandOutput(ctx, workerDir, nil, "d1", "create", dbName)
	if err != nil {
		outInfo, infoErr := runWranglerCommandOutput(ctx, workerDir, nil, "d1", "info", dbName)
		if infoErr != nil {
			return "", err
		}

		id := parseDatabaseID(outInfo)
		if id == "" {
			return "", errParseDatabaseIDInfo
		}

		return id, nil
	}

	id := parseDatabaseID(out)
	if id == "" {
		return "", errParseDatabaseIDCreate
	}

	return id, nil
}

func parseDatabaseID(out string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`database_id\s*=\s*\"([^\"]+)\"`),
		regexp.MustCompile(`database_id\s*:\s*\"?([a-zA-Z0-9-]+)\"?`),
		regexp.MustCompile(`Database ID:\s*([a-zA-Z0-9-]+)`),
	}
	for _, re := range patterns {
		if match := re.FindStringSubmatch(out); len(match) > 1 {
			return match[1]
		}
	}

	return ""
}

func writeWranglerConfig(workerDir, workerName, dbName, dbID string) (string, error) {
	templatePath := filepath.Join(workerDir, "wrangler.toml")
	// #nosec G304 -- path is derived from the configured worker dir
	data, err := os.ReadFile(templatePath)
	if err != nil {
		return "", fmt.Errorf("read wrangler.toml: %w", err)
	}

	content := string(data)
	content = replaceTomlString(content, "name", workerName)
	content = replaceTomlString(content, "database_name", dbName)
	content = replaceTomlString(content, "database_id", dbID)

	tmpFile, err := os.CreateTemp("", "gog-wrangler-*.toml")
	if err != nil {
		return "", fmt.Errorf("create temp wrangler config: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		return "", fmt.Errorf("write temp wrangler config: %w", err)
	}

	return tmpFile.Name(), nil
}

func replaceTomlString(content, key, value string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?m)^%s\s*=\s*\".*\"\s*$`, regexp.QuoteMeta(key)))
	return re.ReplaceAllString(content, fmt.Sprintf(`%s = \"%s\"`, key, value))
}

func runWranglerCommand(ctx context.Context, dir string, stdin io.Reader, args ...string) error {
	_, err := runWranglerCommandOutput(ctx, dir, stdin, args...)

	return err
}

func runWranglerCommandOutput(ctx context.Context, dir string, stdin io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "wrangler", args...) //nolint:gosec // executable is fixed; args are explicit CLI args
	cmd.Dir = dir
	cmd.Stdin = stdin

	cmd.Env = append(os.Environ(), "WRANGLER_SEND_METRICS=false")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("wrangler %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return string(out), nil
}
