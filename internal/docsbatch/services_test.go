package docsbatch

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestOnlySheetsAllowsEmptyRevision(t *testing.T) {
	for _, service := range []string{ServiceDocs, ServiceSlides, ServiceForms, ServiceSheets} {
		t.Run(service, func(t *testing.T) {
			store := New(t.TempDir(), Options{})

			state := State{Service: service, Account: "test@example.com", Client: "default"}
			switch service {
			case ServiceDocs:
				state.DocumentID = "doc1"
			case ServiceSlides:
				state.PresentationID = "deck1"
			case ServiceForms:
				state.FormID = "form1"
				count := 0
				state.InitialFormItems = &count
			case ServiceSheets:
				state.SpreadsheetID = "sheet1"
			}

			created, err := store.Create(state)
			if err != nil {
				t.Fatal(err)
			}
			options := AppendOptions{BatchID: created.BatchID, Identity: Identity{Service: service, DocumentID: state.DocumentID, PresentationID: state.PresentationID, FormID: state.FormID, SpreadsheetID: state.SpreadsheetID, Account: state.Account, Client: state.Client}, Requests: []json.RawMessage{json.RawMessage(`{"request":0}`)}}

			_, emptyErr := store.Append(options)
			if service == ServiceSheets {
				if emptyErr != nil {
					t.Fatal(emptyErr)
				}

				options.RevisionID = "not-supported"
				if _, appendErr := store.Append(options); !errors.Is(appendErr, ErrUnsupportedRevision) {
					t.Fatalf("Sheets accepted a revision: %v", appendErr)
				}
			} else {
				if !errors.Is(emptyErr, ErrEmptyRevision) {
					t.Fatalf("%s lost its revision requirement: %v", service, emptyErr)
				}

				options.RevisionID = "rev1"
				if _, appendErr := store.Append(options); appendErr != nil {
					t.Fatal(appendErr)
				}
			}

			loaded, err := store.Get(created.BatchID)
			if err != nil || len(loaded.Requests) != 1 {
				t.Fatalf("read state: %v %+v", err, loaded)
			}

			if service == ServiceForms && (loaded.InitialFormItems == nil || *loaded.InitialFormItems != 0 || loaded.FormID != "form1") {
				t.Fatalf("lost empty Forms base: %+v", loaded)
			}

			listed, err := store.List()
			if err != nil || len(listed) != 1 || listed[0].FormID != state.FormID || listed[0].SpreadsheetID != state.SpreadsheetID {
				t.Fatalf("lost target in summary: %v %+v", err, listed)
			}
		})
	}
}
