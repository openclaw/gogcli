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
	TrackingKeys map[int]string
	AdminKey     string
	TrackingCurrentVersion int
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

	trackingKeys, currentVersion, trackingErr := resolveTrackingDeploymentSecrets(opts)
	if trackingErr != nil {
		return "", trackingErr
	}

	for _, version := range trackingKeys {
		secretName := fmt.Sprintf("TRACKING_KEY_V%d", version.version)
		if runErr := runWranglerCommand(ctx, workerDir, strings.NewReader(version.key+"\n"), "secret", "put", secretName, "--name", opts.WorkerName); runErr != nil {
			return "", runErr
		}
	}

	if runErr := runWranglerCommand(ctx, workerDir, strings.NewReader(fmt.Sprintf("%d\n", currentVersion)), "secret", "put", "TRACKING_KEY_CURRENT_VERSION", "--name", opts.WorkerName); runErr != nil {
		return "", runErr
	}

	if legacyKey, ok := trackingKeyByVersion(trackingKeys, currentVersion); ok {
		if runErr := runWranglerCommand(ctx, workerDir, strings.NewReader(legacyKey+"\n"), "secret", "put", "TRACKING_KEY", "--name", opts.WorkerName); runErr != nil {
			return "", runErr
		}
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

type trackingDeploymentSecret struct {
	version int
	key     string
}

func resolveTrackingDeploymentSecrets(opts DeployOptions) ([]trackingDeploymentSecret, int, error) {
	if len(opts.TrackingKeys) > 0 {
		versions := sortedTrackingVersions(opts.TrackingKeys)
		if len(versions) == 0 {
			return nil, 0, fmt.Errorf("tracking key map is empty")
		}

		currentVersion := normalizedTrackingCurrentVersion(opts.TrackingCurrentVersion, versions)
		secrets := make([]trackingDeploymentSecret, 0, len(versions))
		for _, version := range versions {
			key := strings.TrimSpace(opts.TrackingKeys[version])
			if key == "" {
				continue
			}
			secrets = append(secrets, trackingDeploymentSecret{version: version, key: key})
		}
		if len(secrets) == 0 {
			return nil, 0, fmt.Errorf("tracking key map is empty")
		}

		return secrets, currentVersion, nil
	}

	if strings.TrimSpace(opts.TrackingKey) == "" {
		return nil, 0, fmt.Errorf("missing tracking key")
	}

	return []trackingDeploymentSecret{
		{
			version: 1,
			key:     opts.TrackingKey,
		},
	}, 1, nil
}

func normalizedTrackingCurrentVersion(current int, versions []int) int {
	if current > 0 {
		for _, version := range versions {
			if version == current {
				return current
			}
		}
		return versions[len(versions)-1]
	}

	return versions[len(versions)-1]
}

func sortedTrackingVersions(keys map[int]string) []int {
	versions := make([]int, 0, len(keys))
	for version := range keys {
		versions = append(versions, version)
	}
	// simple ascending order
	for i := 1; i < len(versions); i++ {
		j := i
		for j > 0 && versions[j-1] > versions[j] {
			versions[j-1], versions[j] = versions[j], versions[j-1]
			j--
		}
	}

	return versions
}

func trackingKeyByVersion(secrets []trackingDeploymentSecret, current int) (string, bool) {
	for _, s := range secrets {
		if s.version == current {
			return s.key, true
		}
	}
	return "", false
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
	cmd := exec.CommandContext(ctx, "wrangler", args...)
	cmd.Dir = dir
	cmd.Stdin = stdin

	cmd.Env = append(os.Environ(), "WRANGLER_SEND_METRICS=false")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("wrangler %s failed: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}

	return string(out), nil
}
