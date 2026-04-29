package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/steipete/gogcli/internal/backup"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type BackupCatCmd struct {
	backupReadFlags
	Shard  string `arg:"" name:"shard" help:"Manifest shard path, or absolute path under the backup repo"`
	Pretty bool   `name:"pretty" help:"Pretty-print each JSONL row"`
	Out    string `name:"out" help:"Write decrypted JSONL to this file instead of stdout"`
}

func (c *BackupCatCmd) Run(ctx context.Context) error {
	shard, err := backup.Cat(ctx, c.options(), c.Shard)
	if err != nil {
		return err
	}
	data := shard.Plaintext
	if c.Pretty {
		data, err = prettyJSONL(data)
		if err != nil {
			return fmt.Errorf("pretty-print shard: %w", err)
		}
	}
	if strings.TrimSpace(c.Out) != "" {
		out, expandErr := expandUserPath(c.Out)
		if expandErr != nil {
			return expandErr
		}
		if mkdirErr := os.MkdirAll(filepath.Dir(out), 0o700); mkdirErr != nil {
			return mkdirErr
		}
		return os.WriteFile(out, data, 0o600)
	}
	_, err = os.Stdout.Write(data)
	return err
}

type BackupExportCmd struct {
	backupReadFlags
	Out              string `name:"out" help:"Plaintext export directory" default:"~/Documents/gog-backup-export"`
	GmailFormat      string `name:"gmail-format" help:"Gmail message export format: eml, markdown, or both" default:"eml" enum:"eml,markdown,both"`
	GmailAttachments string `name:"gmail-attachments" help:"Gmail attachment export mode for markdown/both: extract or none" default:"extract" enum:"extract,none"`
}

type backupExportResult struct {
	Out            string         `json:"out"`
	Repo           string         `json:"repo"`
	ManifestExport time.Time      `json:"manifestExported"`
	Files          int            `json:"files"`
	Counts         map[string]int `json:"counts"`
}

type backupExportOptions struct {
	GmailFormat      string
	GmailAttachments string
}

func (c *BackupExportCmd) Run(ctx context.Context) error {
	outDir, err := expandUserPath(c.Out)
	if err != nil {
		return err
	}
	exportOpts := backupExportOptions{
		GmailFormat:      c.GmailFormat,
		GmailAttachments: c.GmailAttachments,
	}
	result := backupExportResult{
		Out:    outDir,
		Counts: map[string]int{},
	}
	initialized := false
	shardIndex := 0
	u := ui.FromContext(ctx)
	initExport := func(manifest backup.Manifest, repo string) error {
		if initialized {
			return nil
		}
		if exportErr := ensureExportOutsideRepo(outDir, repo); exportErr != nil {
			return exportErr
		}
		result.Repo = repo
		result.ManifestExport = manifest.Exported
		if mkdirErr := os.MkdirAll(outDir, 0o700); mkdirErr != nil {
			return mkdirErr
		}
		if readmeErr := writeBackupExportReadme(outDir); readmeErr != nil {
			return readmeErr
		}
		if manifestErr := writeJSONFile(filepath.Join(outDir, "manifest.json"), manifest); manifestErr != nil {
			return manifestErr
		}
		if resetErr := resetExportTargets(outDir, manifest.Shards); resetErr != nil {
			return resetErr
		}
		initialized = true
		return nil
	}
	var manifest backup.Manifest
	var repo string
	manifest, repo, err = backup.WalkSnapshot(ctx, c.options(), func(snapshot backup.Manifest, snapshotRepo string, shard backup.PlainShard) error {
		if initErr := initExport(snapshot, snapshotRepo); initErr != nil {
			return initErr
		}
		shardIndex++
		if u != nil {
			key := shard.Service
			if strings.TrimSpace(shard.Kind) != "" {
				key += "." + shard.Kind
			}
			u.Err().Printf("export\t%d/%d\t%s\trows=%d", shardIndex, len(snapshot.Shards), key, shard.Rows)
		}
		_, count, shardErr := exportPlainShard(outDir, shard, exportOpts)
		if shardErr != nil {
			return shardErr
		}
		key := shard.Service
		if strings.TrimSpace(shard.Kind) != "" {
			key += "." + shard.Kind
		}
		result.Counts[key] += count
		return nil
	})
	if err != nil {
		return err
	}
	if !initialized {
		if initErr := initExport(manifest, repo); initErr != nil {
			return initErr
		}
	}
	files, err := countExportFiles(outDir)
	if err != nil {
		return err
	}
	result.Files = files
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, result)
	}
	u.Out().Printf("out\t%s", result.Out)
	u.Out().Printf("repo\t%s", result.Repo)
	u.Out().Printf("files\t%d", result.Files)
	keys := make([]string, 0, len(result.Counts))
	for key := range result.Counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		u.Out().Printf("count.%s\t%d", key, result.Counts[key])
	}
	return nil
}

func prettyJSONL(data []byte) ([]byte, error) {
	var out bytes.Buffer
	for _, rawLine := range bytes.Split(data, []byte{'\n'}) {
		trimmedLine := bytes.TrimSpace(rawLine)
		if len(trimmedLine) == 0 {
			continue
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, trimmedLine, "", "  "); err != nil {
			return nil, err
		}
		if _, err := pretty.WriteTo(&out); err != nil {
			return nil, err
		}
		if err := out.WriteByte('\n'); err != nil {
			return nil, err
		}
	}
	return out.Bytes(), nil
}

func expandUserPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "~/Documents/gog-backup-export"
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func ensureExportOutsideRepo(outDir, repo string) error {
	outAbs, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	repoAbs, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	outDir = filepath.Clean(outAbs)
	repo = filepath.Clean(repoAbs)
	rel, err := filepath.Rel(repo, outDir)
	if err != nil {
		return err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".." && !filepath.IsAbs(rel)) {
		return fmt.Errorf("plaintext export directory must be outside backup repo: %s", outDir)
	}
	return nil
}

func resetExportTargets(outDir string, shards []backup.ShardEntry) error {
	seen := map[string]struct{}{}
	for _, shard := range shards {
		target := ""
		switch {
		case shard.Service == backupServiceGmail && shard.Kind == "messages":
			target = filepath.Join(outDir, backupServiceGmail, sanitizeFilePart(shard.Account), "messages", "index.jsonl")
		case shard.Service == backupServiceDrive && shard.Kind == "contents":
			target = filepath.Join(outDir, backupServiceDrive, sanitizeFilePart(shard.Account), "files", "index.jsonl")
		}
		if target == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func writeBackupExportReadme(outDir string) error {
	const body = "# gog backup plaintext export\n" +
		"\n" +
		"This directory is an unencrypted local copy created by `gog backup export`.\n" +
		"Keep it out of Git, shared folders, and cloud sync unless that is intentional.\n" +
		"\n" +
		"Gmail messages are written according to `--gmail-format`: `.eml` by default,\n" +
		"Markdown notes with extracted attachment files when `--gmail-format markdown`,\n" +
		"or both when `--gmail-format both`. `gmail/<account>/messages/index.jsonl`\n" +
		"maps backup message IDs to exported files. Labels are written as pretty JSON.\n"
	return os.WriteFile(filepath.Join(outDir, "README.md"), []byte(body), 0o600)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func exportPlainShard(outDir string, shard backup.PlainShard, opts backupExportOptions) (int, int, error) {
	switch {
	case shard.Service == backupServiceDrive && shard.Kind == "contents":
		return exportDriveContents(outDir, shard)
	case shard.Service == backupServiceGmail && shard.Kind == "labels":
		return exportGmailLabels(outDir, shard)
	case shard.Service == backupServiceGmail && shard.Kind == "messages":
		return exportGmailMessages(outDir, shard, opts)
	default:
		return exportRawShard(outDir, shard)
	}
}

func exportDriveContents(outDir string, shard backup.PlainShard) (int, int, error) {
	var rows []driveBackupContent
	if err := backup.DecodeJSONL(shard.Plaintext, &rows); err != nil {
		return 0, 0, err
	}
	account := sanitizeFilePart(shard.Account)
	indexPath := filepath.Join(outDir, backupServiceDrive, account, "files", "index.jsonl")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o700); err != nil {
		return 0, 0, err
	}
	indexFile, err := os.OpenFile(indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304 -- path is confined to caller-selected export dir and sanitized account.
	if err != nil {
		return 0, 0, err
	}
	defer indexFile.Close()
	enc := json.NewEncoder(indexFile)
	enc.SetEscapeHTML(false)
	files := 0
	for _, row := range rows {
		rel := filepath.ToSlash(filepath.Join(backupServiceDrive, account, "files", sanitizeFilePart(row.FileID), sanitizeFilePart(row.ExportName)))
		indexRow := map[string]any{
			"fileId":         row.FileID,
			"name":           row.Name,
			"mimeType":       row.MimeType,
			"exportName":     row.ExportName,
			"exportMimeType": row.ExportMime,
			"source":         row.Source,
			"size":           row.Size,
			"modifiedTime":   row.ModifiedTime,
			"path":           rel,
			"skipped":        row.Skipped,
			"error":          row.Error,
		}
		if err := enc.Encode(indexRow); err != nil {
			return files, 0, err
		}
		if row.DataBase64 == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(row.DataBase64)
		if err != nil {
			return files, 0, fmt.Errorf("decode Drive content %s: %w", row.FileID, err)
		}
		path := filepath.Join(outDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return files, 0, err
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return files, 0, err
		}
		files++
	}
	return files + 1, len(rows), nil
}

func exportRawShard(outDir string, shard backup.PlainShard) (int, int, error) {
	rel := strings.TrimSuffix(shard.Path, ".gz.age")
	path := filepath.Join(outDir, "raw", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, 0, err
	}
	if err := os.WriteFile(path, shard.Plaintext, 0o600); err != nil {
		return 0, 0, err
	}
	return 1, shard.Rows, nil
}

func countExportFiles(outDir string) (int, error) {
	count := 0
	err := filepath.WalkDir(outDir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d != nil && !d.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

func sanitizeFilePart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return trackingUnknown
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return trackingUnknown
	}
	return out
}
