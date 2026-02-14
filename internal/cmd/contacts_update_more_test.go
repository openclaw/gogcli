package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/steipete/gogcli/internal/ui"
)

func TestContactsUpdate_BirthdayAndNotes_Set(t *testing.T) {
	var gotGetFields string
	var gotUpdateFields string
	var gotBirthday string
	var gotNotes string

	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/c1") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":"):
			gotGetFields = r.URL.Query().Get("personFields")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": "people/c1",
				"names":        []map[string]any{{"givenName": "Ada", "familyName": "Lovelace"}},
			})
			return
		case strings.Contains(r.URL.Path, ":updateContact") && (r.Method == http.MethodPatch || r.Method == http.MethodPost):
			gotUpdateFields = r.URL.Query().Get("updatePersonFields")
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if birthdays, ok := body["birthdays"].([]any); ok && len(birthdays) > 0 {
				if first, ok := birthdays[0].(map[string]any); ok {
					if date, ok := first["date"].(map[string]any); ok {
						gotBirthday = strings.TrimSpace(primaryValue(date, "year") + "-" + leftPad2(primaryValue(date, "month")) + "-" + leftPad2(primaryValue(date, "day")))
					}
				}
			}
			if bios, ok := body["biographies"].([]any); ok && len(bios) > 0 {
				if first, ok := bios[0].(map[string]any); ok {
					gotNotes = strings.TrimSpace(primaryValue(first, "value"))
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceName": "people/c1"})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--birthday", "2026-02-13", "--notes", "note text"}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if !strings.Contains(gotGetFields, "birthdays") || !strings.Contains(gotGetFields, "biographies") {
		t.Fatalf("missing people.get fields: %q", gotGetFields)
	}
	if !strings.Contains(gotUpdateFields, "birthdays") || !strings.Contains(gotUpdateFields, "biographies") {
		t.Fatalf("missing update fields: %q", gotUpdateFields)
	}
	if gotBirthday != "2026-02-13" {
		t.Fatalf("unexpected birthday payload: %q", gotBirthday)
	}
	if gotNotes != "note text" {
		t.Fatalf("unexpected notes payload: %q", gotNotes)
	}
}

func TestContactsUpdate_BirthdayAndNotes_Clear(t *testing.T) {
	var gotUpdateFields string

	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/c1") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resourceName": "people/c1",
				"birthdays":    []map[string]any{{"date": map[string]any{"year": 2026, "month": 2, "day": 13}}},
				"biographies":  []map[string]any{{"value": "existing"}},
			})
			return
		case strings.Contains(r.URL.Path, ":updateContact") && (r.Method == http.MethodPatch || r.Method == http.MethodPost):
			gotUpdateFields = r.URL.Query().Get("updatePersonFields")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceName": "people/c1"})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	u, err := ui.New(ui.Options{Stdout: io.Discard, Stderr: io.Discard, Color: "never"})
	if err != nil {
		t.Fatalf("ui.New: %v", err)
	}
	ctx := ui.WithUI(context.Background(), u)

	if err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--birthday", "", "--notes", ""}, ctx, &RootFlags{Account: "a@b.com"}); err != nil {
		t.Fatalf("runKong: %v", err)
	}

	if !strings.Contains(gotUpdateFields, "birthdays") || !strings.Contains(gotUpdateFields, "biographies") {
		t.Fatalf("missing clear update fields: %q", gotUpdateFields)
	}
}

func TestContactsUpdate_InvalidBirthday(t *testing.T) {
	svc, closeSrv := newPeopleService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "people/c1") && r.Method == http.MethodGet && !strings.Contains(r.URL.Path, ":"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"resourceName": "people/c1"})
			return
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(closeSrv)
	stubPeopleServices(t, svc)

	err := runKong(t, &ContactsUpdateCmd{}, []string{"people/c1", "--birthday", "2026/02/13"}, context.Background(), &RootFlags{Account: "a@b.com"})
	if err == nil || !strings.Contains(err.Error(), "invalid --birthday") {
		t.Fatalf("expected invalid --birthday error, got %v", err)
	}
}

func primaryValue(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch vv := v.(type) {
	case string:
		return vv
	case float64:
		return strconv.Itoa(int(vv))
	case int:
		return strconv.Itoa(vv)
	default:
		return ""
	}
}

func leftPad2(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		return s
	}
	if s == "" {
		return "00"
	}
	return "0" + s
}
