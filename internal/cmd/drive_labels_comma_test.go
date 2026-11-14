package cmd

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"

	"google.golang.org/api/drive/v3"
)

// The field flags are parsed by the CLI, not built in Go: a comma in the
// flag value must reach gog's own parser, so text keeps its comma and the
// documented field=a,b forms produce multiple values.
func TestDriveLabelsFileApply_CommaValuesThroughCLI(t *testing.T) {
	var got []*drive.LabelFieldModification
	svc, closeSvc := newDriveTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/files/file1/modifyLabels" {
			http.NotFound(w, r)
			return
		}
		var req drive.ModifyLabelsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			t.Fatalf("decode body: %v", err)
		}
		got = req.LabelModifications[0].FieldModifications
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"modifiedLabels": []map[string]any{{"id": "label1"}}})
	}))
	defer closeSvc()

	ctx := withDriveTestService(newCmdRuntimeJSONOutputContext(t, io.Discard, io.Discard), svc)
	err := runKong(t, &DriveLabelsFileApplyCmd{}, []string{
		"file1", "label1",
		"--text", "note=Hello, world",
		"--selection", "status=c1,c2",
		"--integer", "count=1,2",
		"--user", "owners=a@example.com,b@example.com",
	}, ctx, &RootFlags{Account: "a@example.com", Force: true})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	byField := map[string]*drive.LabelFieldModification{}
	for _, mod := range got {
		byField[mod.FieldId] = mod
	}
	if v := byField["note"]; v == nil || !reflect.DeepEqual(v.SetTextValues, []string{"Hello, world"}) {
		t.Fatalf("note = %#v, want one text value with its comma", v)
	}
	if v := byField["status"]; v == nil || !reflect.DeepEqual(v.SetSelectionValues, []string{"c1", "c2"}) {
		t.Fatalf("status = %#v, want two selection values", v)
	}
	if v := byField["count"]; v == nil || !reflect.DeepEqual([]int64(v.SetIntegerValues), []int64{1, 2}) {
		t.Fatalf("count = %#v, want two integer values", v)
	}
	if v := byField["owners"]; v == nil || !reflect.DeepEqual(v.SetUserValues, []string{"a@example.com", "b@example.com"}) {
		t.Fatalf("owners = %#v, want two user values", v)
	}
}
