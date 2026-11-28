package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
)

const driveUploadReceiptFlag = "--include-committed-revision"

func TestDriveUpload_ConditionalReceipt(t *testing.T) {
	const etag = `"before-write"`
	const sha256 = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const md5 = "0123456789abcdef0123456789abcdef"

	for _, tc := range []struct {
		name     string
		optIn    bool
		status   int
		complete bool
	}{
		{name: "opt-in receipt", optIn: true, status: http.StatusOK, complete: true},
		{name: "default output", status: http.StatusOK, complete: true},
		{name: "missing committed fields", optIn: true, status: http.StatusOK},
		{name: "competing update", optIn: true, status: http.StatusPreconditionFailed},
		{name: "ambiguous server error", optIn: true, status: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reads, writes int
			svc := newDriveUploadReplaceTestService(t, true, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && strings.TrimPrefix(r.URL.Path, "/drive/v2") == "/files/file1":
					reads++
					_ = json.NewEncoder(w).Encode(map[string]string{
						"id": "file1", "mimeType": "application/pdf", "version": "42", "etag": etag,
						"headRevisionId": "before-head", "sha256Checksum": "before-sha256",
					})
				case r.Method == http.MethodPut && r.URL.Path == "/upload/drive/v2/files/file1":
					writes++
					if got := r.Header.Get("If-Match"); got != etag {
						t.Errorf("If-Match = %q, want %q", got, etag)
					}
					fields := strings.Split(strings.ReplaceAll(r.URL.Query().Get("fields"), " ", ""), ",")
					for _, field := range []string{"headRevisionId", "sha256Checksum", "md5Checksum"} {
						if slices.Contains(fields, field) != tc.optIn {
							t.Errorf("fields = %q, presence of %q should be %t", fields, field, tc.optIn)
						}
					}
					w.WriteHeader(tc.status)
					if tc.status != http.StatusOK {
						return
					}
					reply := map[string]string{"id": "file1", "title": "Existing.pdf", "mimeType": "application/pdf"}
					if tc.complete {
						reply["headRevisionId"], reply["sha256Checksum"], reply["md5Checksum"] = "committed-head", sha256, md5
					}
					_ = json.NewEncoder(w).Encode(reply)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			})
			args := []string{writeDriveUploadReplaceTestFile(t), "--replace", "file1", "--if-version", "42"}
			if tc.optIn {
				args = append(args, driveUploadReceiptFlag)
			}
			var out bytes.Buffer
			ctx := withDriveV2TestService(newCmdRuntimeJSONOutputContext(t, &out, io.Discard), svc)
			err := runKong(t, &DriveUploadCmd{}, args, ctx, &RootFlags{Account: "a@b.com", Force: true})
			if tc.status != http.StatusOK {
				if err == nil || out.Len() != 0 {
					t.Fatalf("failed write produced err=%v, stdout=%q", err, out.Bytes())
				}
			} else {
				if err != nil {
					t.Fatalf("conditional replace: %v", err)
				}
				var got struct {
					File            drive.File `json:"file"`
					Replaced        bool       `json:"replaced"`
					PreservedFileID bool       `json:"preservedFileId"`
				}
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatalf("decode receipt: %v", err)
				}
				if got.File.Id != "file1" || !got.Replaced || !got.PreservedFileID {
					t.Fatalf("unexpected replacement result: %s", out.Bytes())
				}
				if tc.optIn && tc.complete {
					if got.File.HeadRevisionId != "committed-head" || got.File.Sha256Checksum != sha256 || got.File.Md5Checksum != md5 {
						t.Fatalf("unexpected committed receipt: %s", out.Bytes())
					}
				} else if got.File.HeadRevisionId != "" || got.File.Sha256Checksum != "" || got.File.Md5Checksum != "" {
					t.Fatalf("unexpected receipt fields: %s", out.Bytes())
				}
			}
			if reads != 1 || writes != 1 {
				t.Fatalf("got %d reads and %d writes, want one metadata read and one conditional upload", reads, writes)
			}
		})
	}
}

func TestDriveUpload_ConditionalReceiptValidation(t *testing.T) {
	local := writeDriveUploadReplaceTestFile(t)
	for _, tc := range []struct {
		name  string
		args  []string
		error string
	}{
		{"create", []string{local, driveUploadReceiptFlag}, "requires --if-version"},
		{"unguarded replacement", []string{local, "--replace", "file1", driveUploadReceiptFlag}, "requires --if-version"},
		{"no replacement target", []string{local, "--if-version", "42", driveUploadReceiptFlag}, "requires --replace"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard)
			err := runKong(t, &DriveUploadCmd{}, tc.args, ctx, &RootFlags{DryRun: true})
			if ExitCode(err) != 2 || !strings.Contains(err.Error(), tc.error) {
				t.Fatalf("got %v, want usage error containing %q", err, tc.error)
			}
		})
	}

	var out bytes.Buffer
	ctx := newCmdRuntimeJSONOutputContext(t, &out, io.Discard)
	args := []string{local, "--replace", "file1", "--if-version", "42", driveUploadReceiptFlag}
	err := runKong(t, &DriveUploadCmd{}, args, ctx, &RootFlags{DryRun: true})
	if ExitCode(err) != 0 {
		t.Fatalf("dry run: %v", err)
	}
	var got struct {
		Request map[string]any `json:"request"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if got.Request["include_committed_revision"] != true || got.Request["if_version"] != float64(42) {
		t.Fatalf("preview missing conditional receipt intent: %s", out.Bytes())
	}
}
