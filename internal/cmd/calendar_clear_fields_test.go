package cmd

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/alecthomas/kong"
)

func TestCalendarUpdateExplicitEmptyFieldsJSON(t *testing.T) {
	for _, field := range []string{"summary", "location", "description", "attendees"} {
		for _, value := range []string{"", "   "} {
			t.Run(field+"/"+value, func(t *testing.T) {
				cmd := &CalendarUpdateCmd{}
				parser, err := kong.New(cmd, kong.Writers(io.Discard, io.Discard))
				if err != nil {
					t.Fatal(err)
				}
				kctx, err := parser.Parse([]string{"primary", "event1", "--" + field + "=" + value})
				if err != nil {
					t.Fatal(err)
				}
				patch, changed, err := buildCalendarUpdatePatch(calendarUpdateInputFromCommand(cmd), calendarUpdateFieldsFromKong(kctx))
				if err != nil || !changed {
					t.Fatalf("changed=%v err=%v", changed, err)
				}
				data, err := json.Marshal(patch)
				if err != nil {
					t.Fatal(err)
				}
				var body map[string]json.RawMessage
				if err := json.Unmarshal(data, &body); err != nil {
					t.Fatal(err)
				}
				want := `""`
				if field == "attendees" {
					want = "[]"
				}
				if string(body[field]) != want {
					t.Fatalf("body=%s; want %s=%s", data, field, want)
				}
				if len(body) != 1 {
					t.Fatalf("omitted fields leaked into PATCH: %s", data)
				}
			})
		}
	}
}
