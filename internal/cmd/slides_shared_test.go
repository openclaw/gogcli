package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/api/slides/v1"
)

func TestResolveSlidesNotesInput_FileTakesPrecedence(t *testing.T) {
	notesPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notesPath, []byte("from file"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	got, ok, err := resolveSlidesNotesInput(ptrString("from flag"), notesPath)
	if err != nil {
		t.Fatalf("resolve notes: %v", err)
	}
	if !ok || got != "from file" {
		t.Fatalf("resolve notes = %q, %t; want file content", got, ok)
	}
}

func TestFindSpeakerNotesObjectID_FallsBackToBodyPlaceholder(t *testing.T) {
	slide := &slides.Page{
		SlideProperties: &slides.SlideProperties{
			NotesPage: &slides.Page{
				PageElements: []*slides.PageElement{
					{
						ObjectId: "body-placeholder",
						Shape: &slides.Shape{
							Placeholder: &slides.Placeholder{Type: placeholderTypeBody},
						},
					},
				},
			},
		},
	}

	if got := findSpeakerNotesObjectID(slide); got != "body-placeholder" {
		t.Fatalf("speaker notes object ID = %q, want body-placeholder", got)
	}
}
