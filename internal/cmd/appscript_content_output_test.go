package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppScriptContentFileNamesStayInOneField(t *testing.T) {
	const name = "Name\tWith\r\nBreaks"
	for _, mode := range []string{"text", "plain", "json"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"scriptId": "script123", "files": []map[string]any{{"name": name, "type": "SERVER_JS"}},
				})
			}))
			defer srv.Close()
			args := []string{"--account", "a@b.com", "appscript", "content", "script123"}
			if mode != "text" {
				args = append([]string{"--" + mode}, args...)
			}
			result := executeWithAppScriptTestService(t, args, newAppScriptTestService(t, srv))
			if result.err != nil {
				t.Fatal(result.err)
			}
			if mode == "json" {
				var output struct {
					Content struct {
						Files []struct{ Name string } `json:"files"`
					} `json:"content"`
				}
				if err := json.Unmarshal([]byte(result.stdout), &output); err != nil {
					t.Fatal(err)
				}
				if len(output.Content.Files) != 1 || output.Content.Files[0].Name != name {
					t.Fatalf("JSON changed the file name: %s", result.stdout)
				}
			} else if !strings.Contains(result.stdout, "file\tName With\\nBreaks\tSERVER_JS\n") {
				t.Fatalf("unsafe file row: %q", result.stdout)
			}
		})
	}
}
