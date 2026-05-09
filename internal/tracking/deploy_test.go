package tracking

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSanitizeWorkerName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "Test@Example.com", want: "test-example-com"},
		{input: " gog--tracker ", want: "gog-tracker"},
		{input: "___", want: ""},
	}
	for _, tc := range cases {
		if got := SanitizeWorkerName(tc.input); got != tc.want {
			t.Fatalf("SanitizeWorkerName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	long := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if got := SanitizeWorkerName(long); len(got) != 63 {
		t.Fatalf("expected max length 63, got %d (%q)", len(got), got)
	}
}

func TestDefaultWorkerName(t *testing.T) {
	if got := DefaultWorkerName(""); got != "gog-email-tracker" {
		t.Fatalf("unexpected default name: %q", got)
	}

	if got := DefaultWorkerName("  "); got != "gog-email-tracker" {
		t.Fatalf("unexpected whitespace name: %q", got)
	}

	if got := DefaultWorkerName("Test@Example.com"); !strings.HasPrefix(got, "gog-email-tracker-") {
		t.Fatalf("unexpected prefixed name: %q", got)
	}
}

func TestParseDatabaseID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: `database_id = "abc-123"`, want: "abc-123"},
		{input: `database_id: abc-123`, want: "abc-123"},
		{input: `Database ID: abc-123`, want: "abc-123"},
		{input: `database_id: "xyz-789"`, want: "xyz-789"},
		{input: `Database ID: 12345`, want: "12345"},
		{input: `database_id = "with-dash"`, want: "with-dash"},
	}
	for _, tc := range cases {
		if got := parseDatabaseID(tc.input); got != tc.want {
			t.Fatalf("parseDatabaseID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	if got := parseDatabaseID("nope"); got != "" {
		t.Fatalf("expected empty id, got %q", got)
	}
}

func TestReplaceTomlString(t *testing.T) {
	content := strings.Join([]string{
		`name = "old"`,
		`database_name = "old-db"`,
		`database_id = "old-id"`,
	}, "\n")

	content = replaceTomlString(content, "name", "new")
	content = replaceTomlString(content, "database_id", "new-id")

	if !strings.Contains(content, `name = "new"`) {
		t.Fatalf("expected name replacement, got %q", content)
	}

	if !strings.Contains(content, `database_id = "new-id"`) {
		t.Fatalf("expected id replacement, got %q", content)
	}

	if !strings.Contains(content, `database_name = "old-db"`) {
		t.Fatalf("unexpected database_name replacement: %q", content)
	}
}

func TestDeployWorker_MissingWrangler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrangler stub uses shell script")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wrangler.toml"), []byte("name = \"x\"\n"), 0o600); err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}

	t.Setenv("PATH", dir)

	_, err := DeployWorker(context.Background(), nil, DeployOptions{
		WorkerDir:    dir,
		WorkerName:   "worker",
		DatabaseName: "db",
		TrackingKey:  "track",
		AdminKey:     "admin",
	})
	if err == nil || !errors.Is(err, errWranglerNotFound) {
		t.Fatalf("expected wrangler not found error, got %v", err)
	}
}

func TestDeployWorker_MissingConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrangler stub uses shell script")
	}

	dir := t.TempDir()
	wranglerPath := writeWranglerStub(t, dir)
	t.Setenv("PATH", filepath.Dir(wranglerPath))

	_, err := DeployWorker(context.Background(), nil, DeployOptions{
		WorkerDir:    dir,
		WorkerName:   "worker",
		DatabaseName: "db",
		TrackingKey:  "track",
		AdminKey:     "admin",
	})
	if err == nil || !errors.Is(err, errWorkerConfigMissing) {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestDeployWorker_Success(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrangler stub uses shell script")
	}

	dir := t.TempDir()
	writeWranglerFiles(t, dir)
	wranglerPath := writeWranglerStub(t, dir)
	t.Setenv("PATH", filepath.Dir(wranglerPath))

	dbID, err := DeployWorker(context.Background(), nil, DeployOptions{
		WorkerDir:    dir,
		WorkerName:   "worker",
		DatabaseName: "db",
		TrackingKey:  "track",
		AdminKey:     "admin",
	})
	if err != nil {
		t.Fatalf("DeployWorker: %v", err)
	}

	if dbID != "db-create" {
		t.Fatalf("unexpected db id: %q", dbID)
	}
}

func TestEnsureD1Database_ListFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrangler stub uses shell script")
	}

	dir := t.TempDir()
	writeWranglerFiles(t, dir)
	wranglerPath := writeWranglerStub(t, dir)
	t.Setenv("PATH", filepath.Dir(wranglerPath))
	t.Setenv("WRANGLER_CREATE_FAIL", "1")

	dbID, err := ensureD1Database(context.Background(), dir, "db")
	if err != nil {
		t.Fatalf("ensureD1Database: %v", err)
	}

	if dbID != "db-list" {
		t.Fatalf("unexpected db id: %q", dbID)
	}
}

func TestEnsureD1Database_InfoFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("wrangler stub uses shell script")
	}

	dir := t.TempDir()
	writeWranglerFiles(t, dir)
	wranglerPath := writeWranglerStub(t, dir)
	t.Setenv("PATH", filepath.Dir(wranglerPath))
	t.Setenv("WRANGLER_CREATE_FAIL", "1")
	t.Setenv("WRANGLER_LIST_FAIL", "1")

	dbID, err := ensureD1Database(context.Background(), dir, "db")
	if err != nil {
		t.Fatalf("ensureD1Database: %v", err)
	}

	if dbID != "db-info" {
		t.Fatalf("unexpected db id: %q", dbID)
	}
}

func TestWriteWranglerConfig(t *testing.T) {
	dir := t.TempDir()
	writeWranglerFiles(t, dir)

	path, err := writeWranglerConfig(dir, "worker-name", "db-name", "db-id")
	if err != nil {
		t.Fatalf("writeWranglerConfig: %v", err)
	}

	if filepath.Dir(path) != dir {
		t.Fatalf("expected config in worker dir %q, got %q", dir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "worker-name") || strings.Contains(content, "old") {
		t.Fatalf("missing name replacement: %q", content)
	}

	if !strings.Contains(content, "db-name") {
		t.Fatalf("missing database_name replacement: %q", content)
	}

	if !strings.Contains(content, "db-id") {
		t.Fatalf("missing database_id replacement: %q", content)
	}
}

func writeWranglerFiles(t *testing.T, dir string) {
	t.Helper()
	wranglerPath := filepath.Join(dir, "wrangler.toml")

	err := os.WriteFile(wranglerPath, []byte("name = \"old\"\ndatabase_name = \"old\"\ndatabase_id = \"old\"\n"), 0o600)
	if err != nil {
		t.Fatalf("write wrangler.toml: %v", err)
	}

	schemaPath := filepath.Join(dir, "schema.sql")

	err = os.WriteFile(schemaPath, []byte(""), 0o600)
	if err != nil {
		t.Fatalf("write schema.sql: %v", err)
	}
}

func writeWranglerStub(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "wrangler")

	err := os.WriteFile(path, []byte(`#!/bin/sh
set -e
cmd="$1"
shift
case "$cmd" in
  d1)
    sub="$1"
    shift
    case "$sub" in
      create)
        if [ "${WRANGLER_CREATE_FAIL:-}" = "1" ]; then
          echo "create failed" >&2
          exit 1
        fi
        echo 'database_id = "db-create"'
        exit 0
        ;;
      info)
        echo 'database_id = "db-info"'
        exit 0
        ;;
      list)
        if [ "${WRANGLER_LIST_FAIL:-}" = "1" ]; then
          echo "list failed" >&2
          exit 1
        fi
        echo '[{"uuid":"db-list","name":"db"}]'
        exit 0
        ;;
      execute)
        exit 0
        ;;
    esac
    ;;
  secret)
    sub="$1"
    shift
    if [ "$sub" = "put" ]; then
      while read _; do :; done
      exit 0
    fi
    ;;
  deploy)
    exit 0
    ;;
esac
echo "unexpected args" >&2
exit 2
`), 0o700)
	if err != nil {
		t.Fatalf("write wrangler stub: %v", err)
	}

	return path
}
