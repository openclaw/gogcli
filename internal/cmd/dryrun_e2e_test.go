package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDryRunE2E_CommandsSkipAuthAPIAndFileWrites(t *testing.T) {
	imagePath := filepath.Join(t.TempDir(), "dryrun.png")
	if err := os.WriteFile(imagePath, []byte("dry-run image placeholder"), 0o644); err != nil {
		t.Fatalf("write dry-run image: %v", err)
	}
	tokenPath := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(tokenPath, []byte(`{"email":"user@example.com","refresh_token":"redacted"}`), 0o600); err != nil {
		t.Fatalf("write dry-run token: %v", err)
	}
	serviceAccountPath := filepath.Join(t.TempDir(), "service-account.json")
	if err := os.WriteFile(serviceAccountPath, []byte(`{"type":"service_account","client_email":"svc@example.com","client_id":"123"}`), 0o600); err != nil {
		t.Fatalf("write dry-run service account: %v", err)
	}
	markdownPath := filepath.Join(t.TempDir(), "slides.md")
	if err := os.WriteFile(markdownPath, []byte("## Slide\nBody"), 0o600); err != nil {
		t.Fatalf("write dry-run markdown: %v", err)
	}
	outputDir := t.TempDir()
	outputPath := func(name string) string {
		return filepath.Join(outputDir, name)
	}

	cases := []struct {
		name    string
		args    []string
		op      string
		outPath string
	}{
		{
			name: "contacts create",
			args: []string{"contacts", "create", "--given", "Smoke", "--email", "smoke@example.com"},
			op:   "contacts.create",
		},
		{
			name: "contacts update",
			args: []string{"contacts", "update", "people/123", "--given", "Smoke"},
			op:   "contacts.update",
		},
		{
			name: "contacts delete",
			args: []string{"contacts", "delete", "people/123"},
			op:   "contacts.delete",
		},
		{
			name: "docs insert",
			args: []string{"docs", "insert", "doc123", "hello"},
			op:   "docs.insert",
		},
		{
			name: "docs clear",
			args: []string{"docs", "clear", "doc123"},
			op:   "docs.clear",
		},
		{
			name: "docs copy",
			args: []string{"docs", "copy", "doc123", "SmokeDoc"},
			op:   "docs.copy",
		},
		{
			name: "docs write replace",
			args: []string{"docs", "write", "doc123", "--text", "hello", "--replace"},
			op:   "docs.write",
		},
		{
			name: "docs write append",
			args: []string{"docs", "write", "doc123", "--text", "hello", "--append"},
			op:   "docs.write",
		},
		{
			name: "docs update",
			args: []string{"docs", "update", "doc123", "--text", "hello", "--index", "1"},
			op:   "docs.update",
		},
		{
			name: "docs delete",
			args: []string{"docs", "delete", "doc123", "--start", "1", "--end", "2"},
			op:   "docs.delete",
		},
		{
			name: "docs format",
			args: []string{"docs", "format", "doc123", "--spacing-mode", "collapse_lists"},
			op:   "docs.format",
		},
		{
			name: "docs find replace",
			args: []string{"docs", "find-replace", "doc123", "old", "new"},
			op:   "docs.find-replace",
		},
		{
			name: "docs add tab",
			args: []string{"docs", "add-tab", "doc123", "--title", "Tab 1"},
			op:   "docs.add-tab",
		},
		{
			name: "docs add child tab",
			args: []string{"docs", "add-tab", "doc123", "--title", "Child", "--parent-tab", "Parent"},
			op:   "docs.add-tab",
		},
		{
			name: "docs rename tab",
			args: []string{"docs", "rename-tab", "doc123", "--tab", "Old", "--title", "New"},
			op:   "docs.rename-tab",
		},
		{
			name: "docs delete tab",
			args: []string{"docs", "delete-tab", "doc123", "--tab", "Old"},
			op:   "docs.delete-tab",
		},
		{
			name: "docs named range create",
			args: []string{"docs", "named-range", "create", "doc123", "--name", "Intro", "--start", "1", "--end", "5"},
			op:   "docs.named-range.create",
		},
		{
			name: "docs named range replace",
			args: []string{"docs", "named-range", "replace", "doc123", "nr123", "--text", "hello"},
			op:   "docs.named-range.replace",
		},
		{
			name: "docs named range delete",
			args: []string{"docs", "named-range", "delete", "doc123", "nr123"},
			op:   "docs.named-range.delete",
		},
		{
			name: "docs export tab",
			args: []string{"docs", "export", "doc123", "--tab", "Tab 1", "--format", "pdf", "--out", "/tmp/gog-dryrun-tab.pdf"},
			op:   "docs.tab-export",
		},
		{
			name: "docs comments delete",
			args: []string{"docs", "comments", "delete", "doc123", "comment123"},
			op:   "docs.comments.delete",
		},
		{
			name: "drive mkdir",
			args: []string{"drive", "mkdir", "SmokeFolder", "--parent", "root"},
			op:   "drive.mkdir",
		},
		{
			name: "drive copy",
			args: []string{"drive", "copy", "file123", "SmokeFile"},
			op:   "drive.copy",
		},
		{
			name: "drive delete",
			args: []string{"drive", "delete", "file123"},
			op:   "drive.delete",
		},
		{
			name: "drive move",
			args: []string{"drive", "move", "file123", "--parent", "folder123"},
			op:   "drive.move",
		},
		{
			name: "drive shortcut create",
			args: []string{"drive", "shortcut", "create", "file123", "--parent", "folder123"},
			op:   "drive.shortcut.create",
		},
		{
			name: "drive rename",
			args: []string{"drive", "rename", "file123", "New"},
			op:   "drive.rename",
		},
		{
			name: "drive upload",
			args: []string{"drive", "upload", imagePath},
			op:   "drive.upload",
		},
		{
			name: "drive share user",
			args: []string{"drive", "share", "file123", "--to", "user", "--email", "user@example.com"},
			op:   "drive.share",
		},
		{
			name: "drive share anyone",
			args: []string{"drive", "share", "file123", "--to", "anyone"},
			op:   "drive.share",
		},
		{
			name: "drive unshare",
			args: []string{"drive", "unshare", "file123", "perm123"},
			op:   "drive.unshare",
		},
		{
			name: "drive download tab",
			args: []string{"drive", "download", "doc123", "--tab", "Tab 1", "--format", "pdf", "--out", "/tmp/gog-dryrun-drive-tab.pdf"},
			op:   "docs.tab-export",
		},
		{
			name:    "drive download",
			args:    []string{"drive", "download", "file123", "--out", outputPath("drive.bin")},
			op:      "drive.download",
			outPath: outputPath("drive.bin"),
		},
		{
			name: "drive changes watch",
			args: []string{"drive", "changes", "watch", "--token", "token123", "--webhook-url", "https://example.com/hook", "--channel-id", "channel123"},
			op:   "drive.changes.watch",
		},
		{
			name: "drive changes stop",
			args: []string{"drive", "changes", "stop", "channel123", "resource123"},
			op:   "drive.changes.stop",
		},
		{
			name: "drive comments delete",
			args: []string{"drive", "comments", "delete", "file123", "comment123"},
			op:   "drive.comments.delete",
		},
		{
			name: "drive bulk remove public",
			args: []string{"drive", "bulk", "remove-public", "--file", "file123"},
			op:   "drive.bulk.remove-public",
		},
		{
			name: "drive bulk update role",
			args: []string{"drive", "bulk", "update-role", "--file", "file123", "--from", "writer", "--to", "reader"},
			op:   "drive.bulk.update-role",
		},
		{
			name: "auth remove",
			args: []string{"auth", "remove", "user@example.com"},
			op:   "auth.remove",
		},
		{
			name: "auth tokens delete",
			args: []string{"auth", "tokens", "delete", "user@example.com"},
			op:   "auth.tokens.delete",
		},
		{
			name: "auth tokens import",
			args: []string{"auth", "tokens", "import", tokenPath},
			op:   "auth.tokens.import",
		},
		{
			name:    "auth tokens export",
			args:    []string{"auth", "tokens", "export", "user@example.com", "--out", outputPath("token-export.json")},
			op:      "auth.tokens.export",
			outPath: outputPath("token-export.json"),
		},
		{
			name: "auth service account set",
			args: []string{"auth", "service-account", "set", "user@example.com", "--key", serviceAccountPath},
			op:   "auth.service-account.set",
		},
		{
			name: "auth service account unset",
			args: []string{"auth", "service-account", "unset", "user@example.com"},
			op:   "auth.service-account.unset",
		},
		{
			name: "auth keep",
			args: []string{"auth", "keep", "user@example.com", "--key", serviceAccountPath},
			op:   "auth.keep",
		},
		{
			name: "admin groups members add",
			args: []string{"admin", "groups", "members", "add", "group@example.com", "user@example.com"},
			op:   "admin.groups.members.add",
		},
		{
			name: "admin groups members remove",
			args: []string{"admin", "groups", "members", "remove", "group@example.com", "user@example.com"},
			op:   "admin.groups.members.remove",
		},
		{
			name: "admin orgunits create",
			args: []string{"admin", "orgunits", "create", "Engineering", "--parent", "/"},
			op:   "admin.orgunits.create",
		},
		{
			name: "admin orgunits update",
			args: []string{"admin", "orgunits", "update", "/Engineering", "--name", "Eng"},
			op:   "admin.orgunits.update",
		},
		{
			name: "admin orgunits delete",
			args: []string{"admin", "orgunits", "delete", "/Engineering"},
			op:   "admin.orgunits.delete",
		},
		{
			name: "admin users create",
			args: []string{"admin", "users", "create", "user@example.com", "--given", "Test", "--family", "User"},
			op:   "admin.users.create",
		},
		{
			name: "admin users delete",
			args: []string{"admin", "users", "delete", "user@example.com"},
			op:   "admin.users.delete",
		},
		{
			name: "admin users suspend",
			args: []string{"admin", "users", "suspend", "user@example.com"},
			op:   "admin.users.suspend",
		},
		{
			name: "calendar create-calendar",
			args: []string{"calendar", "create-calendar", "SmokeCal", "--timezone", "UTC"},
			op:   "calendar.create-calendar",
		},
		{
			name: "calendar subscribe",
			args: []string{"calendar", "subscribe", "other@example.com"},
			op:   "calendar.subscribe",
		},
		{
			name: "calendar unsubscribe",
			args: []string{"calendar", "unsubscribe", "other@example.com"},
			op:   "calendar.unsubscribe",
		},
		{
			name: "calendar delete-calendar",
			args: []string{"calendar", "delete-calendar", "owned@example.com"},
			op:   "calendar.delete-calendar",
		},
		{
			name: "calendar alias set",
			args: []string{"calendar", "alias", "set", "family", "family@group.calendar.google.com"},
			op:   "calendar.alias.set",
		},
		{
			name: "calendar alias unset",
			args: []string{"calendar", "alias", "unset", "family"},
			op:   "calendar.alias.unset",
		},
		{
			name: "calendar focus time",
			args: []string{"calendar", "focus-time", "primary", "--from", "2030-01-01T10:00:00Z", "--to", "2030-01-01T11:00:00Z"},
			op:   "calendar.focus-time",
		},
		{
			name: "calendar out of office",
			args: []string{"calendar", "out-of-office", "primary", "--from", "2030-01-01T10:00:00Z", "--to", "2030-01-01T11:00:00Z"},
			op:   "calendar.out-of-office",
		},
		{
			name: "calendar working location",
			args: []string{"calendar", "working-location", "primary", "--from", "2030-01-01", "--to", "2030-01-02", "--type", "home"},
			op:   "calendar.working-location",
		},
		{
			name: "calendar propose time",
			args: []string{"calendar", "propose-time", "primary", "event123", "--comment", "not this time"},
			op:   "calendar.propose-time",
		},
		{
			name: "calendar respond",
			args: []string{"calendar", "respond", "primary", "event123", "--status", "accepted"},
			op:   "calendar.respond",
		},
		{
			name: "calendar move",
			args: []string{"calendar", "move", "primary", "event123", "other@example.com"},
			op:   "calendar.move",
		},
		{
			name: "forms create",
			args: []string{"forms", "create", "--title", "SmokeForm"},
			op:   "forms.create",
		},
		{
			name: "forms add question",
			args: []string{"forms", "add-question", "form123", "--title", "Question", "--type", "text"},
			op:   "forms.add-question",
		},
		{
			name: "forms publish",
			args: []string{"forms", "publish", "form123"},
			op:   "forms.publish",
		},
		{
			name: "forms watch create",
			args: []string{"forms", "watch", "create", "form123", "--topic", "projects/p/topics/t"},
			op:   "forms.watches.create",
		},
		{
			name: "forms watch delete",
			args: []string{"forms", "watch", "delete", "form123", "watch123"},
			op:   "forms.watches.delete",
		},
		{
			name: "forms watch renew",
			args: []string{"forms", "watch", "renew", "form123", "watch123"},
			op:   "forms.watches.renew",
		},
		{
			name: "forms move question",
			args: []string{"forms", "move-question", "form123", "0", "1"},
			op:   "forms.move-question",
		},
		{
			name: "gmail label rename",
			args: []string{"gmail", "labels", "rename", "Label_1", "NewLabel"},
			op:   "gmail.labels.rename",
		},
		{
			name: "gmail label style",
			args: []string{"gmail", "labels", "style", "Label_1", "--background-color", "#ffffff", "--text-color", "#000000"},
			op:   "gmail.labels.style",
		},
		{
			name: "gmail label delete",
			args: []string{"gmail", "labels", "delete", "Label_1"},
			op:   "gmail.labels.delete",
		},
		{
			name: "gmail label create",
			args: []string{"gmail", "labels", "create", "SmokeLabel"},
			op:   "gmail.labels.create",
		},
		{
			name: "gmail label modify",
			args: []string{"gmail", "labels", "modify", "thread123", "--add", "STARRED"},
			op:   "gmail.labels.modify",
		},
		{
			name: "gmail delegates remove",
			args: []string{"gmail", "delegates", "remove", "delegate@example.com"},
			op:   "gmail.delegates.remove",
		},
		{
			name: "gmail batch delete",
			args: []string{"gmail", "batch", "delete", "msg123", "msg456"},
			op:   "gmail.batch.delete",
		},
		{
			name: "gmail drafts delete",
			args: []string{"gmail", "drafts", "delete", "draft123"},
			op:   "gmail.drafts.delete",
		},
		{
			name: "gmail filters delete",
			args: []string{"gmail", "filters", "delete", "filter123"},
			op:   "gmail.filters.delete",
		},
		{
			name: "gmail sendas delete",
			args: []string{"gmail", "sendas", "delete", "alias@example.com"},
			op:   "gmail.sendas.delete",
		},
		{
			name: "gmail forwarding delete",
			args: []string{"gmail", "forwarding", "delete", "forward@example.com"},
			op:   "gmail.forwarding.delete",
		},
		{
			name: "gmail watch renew",
			args: []string{"gmail", "watch", "renew", "--ttl", "1h"},
			op:   "gmail.watch.renew",
		},
		{
			name: "gmail watch stop",
			args: []string{"gmail", "watch", "stop"},
			op:   "gmail.watch.stop",
		},
		{
			name: "gmail track key rotate",
			args: []string{"gmail", "track", "key", "rotate"},
			op:   "gmail.track.key.rotate",
		},
		{
			name: "keep delete",
			args: []string{"keep", "delete", "note123"},
			op:   "keep.delete",
		},
		{
			name: "tasks delete",
			args: []string{"tasks", "delete", "list123", "task123"},
			op:   "tasks.delete",
		},
		{
			name: "tasks clear",
			args: []string{"tasks", "clear", "list123"},
			op:   "tasks.clear",
		},
		{
			name: "classroom topics delete",
			args: []string{"classroom", "topics", "delete", "course123", "topic123"},
			op:   "classroom.topics.delete",
		},
		{
			name: "classroom coursework delete",
			args: []string{"classroom", "coursework", "delete", "course123", "work123"},
			op:   "classroom.coursework.delete",
		},
		{
			name: "classroom materials delete",
			args: []string{"classroom", "materials", "delete", "course123", "material123"},
			op:   "classroom.materials.delete",
		},
		{
			name: "classroom announcements delete",
			args: []string{"classroom", "announcements", "delete", "course123", "announcement123"},
			op:   "classroom.announcements.delete",
		},
		{
			name: "classroom courses delete",
			args: []string{"classroom", "courses", "delete", "course123"},
			op:   "classroom.courses.delete",
		},
		{
			name: "classroom courses leave",
			args: []string{"classroom", "courses", "leave", "course123", "--role", "student", "--user", "student@example.com"},
			op:   "classroom.courses.leave",
		},
		{
			name: "classroom students remove",
			args: []string{"classroom", "students", "remove", "course123", "student@example.com"},
			op:   "classroom.students.remove",
		},
		{
			name: "classroom teachers remove",
			args: []string{"classroom", "teachers", "remove", "course123", "teacher@example.com"},
			op:   "classroom.teachers.remove",
		},
		{
			name: "classroom invitations delete",
			args: []string{"classroom", "invitations", "delete", "invitation123"},
			op:   "classroom.invitations.delete",
		},
		{
			name: "classroom guardians delete",
			args: []string{"classroom", "guardians", "delete", "student@example.com", "guardian123"},
			op:   "classroom.guardians.delete",
		},
		{
			name: "classroom guardian invitations create",
			args: []string{"classroom", "guardian-invitations", "create", "student@example.com", "--email", "guardian@example.com"},
			op:   "classroom.guardian-invitations.create",
		},
		{
			name: "meet update",
			args: []string{"meet", "update", "abc-defg-hij", "--access", "open"},
			op:   "meet.spaces.patch",
		},
		{
			name: "meet end",
			args: []string{"meet", "end", "abc-defg-hij"},
			op:   "meet.end",
		},
		{
			name: "slides create",
			args: []string{"slides", "create", "SmokeSlides"},
			op:   "slides.create",
		},
		{
			name: "slides copy",
			args: []string{"slides", "copy", "pres123", "SmokeSlides"},
			op:   "slides.copy",
		},
		{
			name: "slides create from template",
			args: []string{"slides", "create-from-template", "template123", "SmokeSlides", "--replace", "NAME=World"},
			op:   "slides.create-from-template",
		},
		{
			name: "slides create from markdown",
			args: []string{"slides", "create-from-markdown", "SmokeSlides", "--content-file", markdownPath},
			op:   "slides.create-from-markdown",
		},
		{
			name: "slides add slide",
			args: []string{"slides", "add-slide", "pres123", imagePath, "--notes", "notes"},
			op:   "slides.add-slide",
		},
		{
			name: "slides delete slide",
			args: []string{"slides", "delete-slide", "pres123", "slide123"},
			op:   "slides.delete-slide",
		},
		{
			name: "slides replace slide",
			args: []string{"slides", "replace-slide", "pres123", "slide123", imagePath, "--notes", "notes"},
			op:   "slides.replace-slide",
		},
		{
			name: "slides update notes",
			args: []string{"slides", "update-notes", "pres123", "slide123", "--notes", "notes"},
			op:   "slides.update-notes",
		},
		{
			name: "slides insert text",
			args: []string{"slides", "insert-text", "pres123", "shape123", "hello"},
			op:   "slides.insert-text",
		},
		{
			name: "slides style text",
			args: []string{"slides", "style-text", "pres123", "shape123", "--range", "0:5", "--bold"},
			op:   "slides.style-text",
		},
		{
			name: "slides link",
			args: []string{"slides", "link", "pres123", "shape123", "--range", "0:5", "--url", "https://example.com"},
			op:   "slides.link",
		},
		{
			name: "slides bullets",
			args: []string{"slides", "bullets", "pres123", "shape123", "--range", "0:5", "--on"},
			op:   "slides.bullets",
		},
		{
			name: "slides replace text",
			args: []string{"slides", "replace-text", "pres123", "old", "new", "--all"},
			op:   "slides.replace-text",
		},
		{
			name: "slides replace text object scoped",
			args: []string{"slides", "replace-text", "pres123", "old", "new", "--object", "shape123"},
			op:   "slides.replace-text",
		},
		{
			name: "appscript create",
			args: []string{"appscript", "create", "--title", "SmokeScript"},
			op:   "appscript.create",
		},
		{
			name: "sheets banding clear all",
			args: []string{"sheets", "banding", "clear", "sheet123", "--sheet", "Sheet1", "--all"},
			op:   "sheets.banding.clear",
		},
		{
			name: "sheets conditional clear index",
			args: []string{"sheets", "conditional-format", "clear", "sheet123", "--sheet", "Sheet1", "--index", "0"},
			op:   "sheets.conditional-format.clear",
		},
		{
			name: "sheets conditional clear all",
			args: []string{"sheets", "conditional-format", "clear", "sheet123", "--sheet", "Sheet1", "--all"},
			op:   "sheets.conditional-format.clear",
		},
		{
			name: "sheets validation set",
			args: []string{"sheets", "validation", "set", "sheet123", "Sheet1!A1:A10", "--type", "ONE_OF_LIST", "--value", "one", "--value", "two"},
			op:   "sheets.validation.set",
		},
		{
			name: "sheets validation clear",
			args: []string{"sheets", "validation", "clear", "sheet123", "Sheet1!A1:A10"},
			op:   "sheets.validation.clear",
		},
		{
			name: "sheets filter set",
			args: []string{"sheets", "filter", "set", "sheet123", "Sheet1!A1:D10"},
			op:   "sheets.filter.set",
		},
		{
			name: "sheets copy",
			args: []string{"sheets", "copy", "sheet123", "SmokeSheet"},
			op:   "sheets.copy",
		},
		{
			name: "sheets table delete",
			args: []string{"sheets", "table", "delete", "sheet123", "Tbl"},
			op:   "sheets.table.delete",
		},
		{
			name: "sheets table append",
			args: []string{"sheets", "table", "append", "sheet123", "Tbl", "a|b"},
			op:   "sheets.table.append",
		},
		{
			name: "sheets table clear",
			args: []string{"sheets", "table", "clear", "sheet123", "Tbl"},
			op:   "sheets.table.clear",
		},
		{
			name: "sheets named ranges add",
			args: []string{"sheets", "named-ranges", "add", "sheet123", "MyRange", "Sheet1!A1:B2"},
			op:   "sheets.named-ranges.add",
		},
		{
			name: "sheets named ranges update",
			args: []string{"sheets", "named-ranges", "update", "sheet123", "range123", "--name", "NewRange"},
			op:   "sheets.named-ranges.update",
		},
		{
			name: "sheets named ranges delete",
			args: []string{"sheets", "named-ranges", "delete", "sheet123", "range123"},
			op:   "sheets.named-ranges.delete",
		},
		{
			name: "sheets delete tab",
			args: []string{"sheets", "delete-tab", "sheet123", "Sheet1"},
			op:   "sheets.delete-tab",
		},
		{
			name: "forms delete question",
			args: []string{"forms", "delete-question", "form123", "0"},
			op:   "forms.delete-question",
		},
		{
			name:    "contacts export",
			args:    []string{"contacts", "export", "people/123", "--out", outputPath("contacts.vcf")},
			op:      "contacts.export",
			outPath: outputPath("contacts.vcf"),
		},
		{
			name:    "slides thumbnail",
			args:    []string{"slides", "thumbnail", "presentation123", "slide123", "--out", outputPath("thumbnail.png")},
			op:      "slides.thumbnail",
			outPath: outputPath("thumbnail.png"),
		},
		{
			name:    "backup cat",
			args:    []string{"backup", "cat", "data/test.jsonl.gz.age", "--repo", outputPath("backup-repo"), "--no-pull", "--out", outputPath("backup.jsonl")},
			op:      "backup.cat",
			outPath: outputPath("backup.jsonl"),
		},
		{
			name:    "backup export",
			args:    []string{"backup", "export", "--repo", outputPath("backup-repo"), "--no-pull", "--out", outputPath("backup-export")},
			op:      "backup.export",
			outPath: outputPath("backup-export"),
		},
		{
			name:    "gmail filters export",
			args:    []string{"gmail", "filters", "export", "--format", "xml", "--out", outputPath("filters.xml")},
			op:      "gmail.filters.export",
			outPath: outputPath("filters.xml"),
		},
		{
			name:    "photos download",
			args:    []string{"photos", "download", "media123", "--out", outputPath("photo.jpg")},
			op:      "photos.download",
			outPath: outputPath("photo.jpg"),
		},
		{
			name:    "photos picker download",
			args:    []string{"photos", "picker", "download", "session123", "media123", "--out", outputPath("picked.jpg")},
			op:      "photos.picker.download",
			outPath: outputPath("picked.jpg"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--dry-run", "--no-input", "--access-token", "invalid-token"}, tc.args...)
			var stderr string
			out := captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					if err := Execute(args); err != nil && ExitCode(err) != 0 {
						t.Fatalf("Execute: %v", err)
					}
				})
			})
			if stderr != "" {
				t.Fatalf("dry-run touched auth/API stderr: %q", stderr)
			}

			var payload struct {
				DryRun  bool            `json:"dry_run"`
				Op      string          `json:"op"`
				Request json.RawMessage `json:"request"`
			}
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatalf("decode dry-run output: %v\nout=%q", err, out)
			}
			if !payload.DryRun || payload.Op != tc.op {
				t.Fatalf("unexpected dry-run output: %#v", payload)
			}
			if len(payload.Request) == 0 || string(payload.Request) == "null" {
				t.Fatalf("dry-run output missing structured request: %s", out)
			}
			if tc.outPath != "" {
				if _, statErr := os.Stat(tc.outPath); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("dry-run wrote output path %s: %v", tc.outPath, statErr)
				}
			}
		})
	}
}

func TestDryRunE2E_CommandSpecificPayloads(t *testing.T) {
	markdownPath := filepath.Join(t.TempDir(), "deck.md")
	if err := os.WriteFile(markdownPath, []byte("---\ntitle: x\n---\n# hi\n"), 0o600); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	cases := []struct {
		name  string
		args  []string
		check func(t *testing.T, request map[string]any)
	}{
		{
			name: "drive upload convert previews remote name",
			args: []string{"drive", "upload", markdownPath, "--convert"},
			check: func(t *testing.T, request map[string]any) {
				t.Helper()
				if got := request["name"]; got != "deck" {
					t.Fatalf("expected stripped remote name deck, got %#v", got)
				}
			},
		},
		{
			name: "gmail track key rotate previews default worker dir",
			args: []string{"gmail", "track", "key", "rotate"},
			check: func(t *testing.T, request map[string]any) {
				t.Helper()
				want := filepath.Join("internal", "tracking", "worker")
				if got := request["worker_dir"]; got != want {
					t.Fatalf("expected worker_dir %q, got %#v", want, got)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--dry-run", "--no-input", "--access-token", "invalid-token"}, tc.args...)
			var stderr string
			out := captureStdout(t, func() {
				stderr = captureStderr(t, func() {
					if err := Execute(args); err != nil && ExitCode(err) != 0 {
						t.Fatalf("Execute: %v", err)
					}
				})
			})
			if stderr != "" {
				t.Fatalf("dry-run touched auth/API stderr: %q", stderr)
			}

			var payload struct {
				Request map[string]any `json:"request"`
			}
			if err := json.Unmarshal([]byte(out), &payload); err != nil {
				t.Fatalf("decode dry-run output: %v\nout=%q", err, out)
			}
			tc.check(t, payload.Request)
		})
	}
}

func TestDryRunE2E_AdminUserCreateDoesNotEmitPassword(t *testing.T) {
	args := []string{
		"--json", "--dry-run", "--no-input", "--access-token", "invalid-token",
		"admin", "users", "create", "user@example.com",
		"--given", "Test", "--family", "User", "--password", "Secret123!",
	}
	var stderr string
	out := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := Execute(args); err != nil && ExitCode(err) != 0 {
				t.Fatalf("Execute: %v", err)
			}
		})
	})
	if stderr != "" {
		t.Fatalf("dry-run touched auth/API stderr: %q", stderr)
	}
	if strings.Contains(out, "Secret123!") {
		t.Fatalf("dry-run output leaked password: %s", out)
	}
	if !strings.Contains(out, `"password": "provided"`) {
		t.Fatalf("dry-run output missing redacted password state: %s", out)
	}
}

func TestDryRunE2E_ValidatesFormsAndSheetsLocalInputs(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "forms add choice requires options before auth",
			args: []string{"forms", "add-question", "form123", "--title", "Q", "--type", "radio"},
		},
		{
			name: "forms add scale rejects invalid lower bound",
			args: []string{"forms", "add-question", "form123", "--title", "Q", "--type", "scale", "--scale-low", "2"},
		},
		{
			name: "forms add scale rejects invalid upper bound",
			args: []string{"forms", "add-question", "form123", "--title", "Q", "--type", "scale", "--scale-high", "11"},
		},
		{
			name: "forms update requires a field before auth",
			args: []string{"forms", "update", "form123"},
		},
		{
			name: "forms update validates quiz before dry-run",
			args: []string{"forms", "update", "form123", "--quiz", "maybe"},
		},
		{
			name: "sheets conditional clear validates index before auth",
			args: []string{"sheets", "conditional-format", "clear", "sheet123", "--sheet", "Sheet1", "--index", "-1"},
		},
		{
			name: "docs write validates format flags before dry-run",
			args: []string{"docs", "write", "doc123", "--text", "hello", "--font-size", "-1"},
		},
		{
			name: "docs write validates spacing mode before dry-run",
			args: []string{"docs", "write", "doc123", "--text", "hello", "--spacing-mode", "sometimes"},
		},
		{
			name: "docs format validates colors before dry-run",
			args: []string{"docs", "format", "doc123", "--text-color", "nope"},
		},
		{
			name: "docs format validates spacing mode before dry-run",
			args: []string{"docs", "format", "doc123", "--spacing-mode", "sometimes"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--dry-run", "--no-input", "--access-token", "invalid-token"}, tc.args...)
			_ = captureStdout(t, func() {
				_ = captureStderr(t, func() {
					if err := Execute(args); ExitCode(err) == 0 {
						t.Fatalf("expected validation failure")
					}
				})
			})
		})
	}
}

func TestDryRunE2E_ContactsUpdateValidatesLocalInputs(t *testing.T) {
	tempDir := t.TempDir()
	malformed := tempDir + "/malformed.json"
	unsupported := tempDir + "/unsupported.json"
	mismatch := tempDir + "/mismatch.json"
	valid := tempDir + "/valid.json"
	for path, body := range map[string]string{
		malformed:   "{",
		unsupported: `{"notAContactField":true}`,
		mismatch:    `{"resourceName":"people/other","names":[{"givenName":"Dry"}]}`,
		valid:       `{"resourceName":"people/123","names":[{"givenName":"Dry"}]}`,
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}

	invalidCases := []struct {
		name string
		args []string
	}{
		{
			name: "bad birthday",
			args: []string{"contacts", "update", "people/123", "--birthday", "nope"},
		},
		{
			name: "bad custom",
			args: []string{"contacts", "update", "people/123", "--custom", "bad"},
		},
		{
			name: "bad relation",
			args: []string{"contacts", "update", "people/123", "--relation", "bad"},
		},
		{
			name: "malformed from-file",
			args: []string{"contacts", "update", "people/123", "--from-file", malformed},
		},
		{
			name: "unsupported from-file key",
			args: []string{"contacts", "update", "people/123", "--from-file", unsupported},
		},
		{
			name: "resource mismatch from-file",
			args: []string{"contacts", "update", "people/123", "--from-file", mismatch},
		},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"--json", "--dry-run", "--no-input", "--access-token", "invalid-token"}, tc.args...)
			_ = captureStdout(t, func() {
				_ = captureStderr(t, func() {
					if err := Execute(args); ExitCode(err) == 0 {
						t.Fatalf("expected validation failure")
					}
				})
			})
		})
	}

	t.Run("valid from-file skips auth and API", func(t *testing.T) {
		args := []string{"--json", "--dry-run", "--no-input", "--access-token", "invalid-token", "contacts", "update", "people/123", "--from-file", valid}
		out := captureStdout(t, func() {
			_ = captureStderr(t, func() {
				if err := Execute(args); err != nil && ExitCode(err) != 0 {
					t.Fatalf("Execute: %v", err)
				}
			})
		})

		var payload struct {
			DryRun bool   `json:"dry_run"`
			Op     string `json:"op"`
		}
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("decode dry-run output: %v\nout=%q", err, out)
		}
		if !payload.DryRun || payload.Op != "contacts.update" {
			t.Fatalf("unexpected dry-run output: %#v", payload)
		}
	})
}
