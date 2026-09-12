package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/api/docs/v1"

	"github.com/openclaw/gogcli/internal/app"
	"github.com/openclaw/gogcli/internal/googleapi"
)

// Use normal command bootstrap so live reads receive the same auth dependencies
// and guards as the CLI. A failed read must fail verification, never skip it.
func readLiveVerificationDocument(account, docID string, runtime *app.Runtime) (*docs.Document, error) {
	var output bytes.Buffer
	runtime.IO = app.IO{In: strings.NewReader(""), Out: &output, Err: io.Discard}
	if err := executeWithRuntime([]string{"--account", account, "--readonly", "--no-input", "--json", "docs", "raw", docID}, runtime); err != nil {
		return nil, err
	}
	var doc docs.Document
	if err := json.Unmarshal(output.Bytes(), &doc); err != nil {
		return nil, fmt.Errorf("decode live document: %w", err)
	}
	return &doc, nil
}

func TestLiveVerificationDocumentRead(t *testing.T) {
	want := &docs.Document{DocumentId: "fixture", Body: &docs.Body{Content: []*docs.StructuralElement{{StartIndex: 1, Paragraph: &docs.Paragraph{Elements: []*docs.ParagraphElement{{TextRun: &docs.TextRun{Content: "proof\n"}}}}}}}}
	svc := newDocsDocumentTestService(t, want, nil)
	runtime := &app.Runtime{KeyringOptions: testKeyringOptions(), Services: app.Services{Docs: func(ctx context.Context, account string) (*docs.Service, error) {
		require.True(t, googleapi.ReadOnly(ctx))
		require.Equal(t, "proof@example.com", account)
		return svc, nil
	}}}
	got, err := readLiveVerificationDocument("proof@example.com", "fixture", runtime)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLiveVerificationReadFailure(t *testing.T) {
	want := errors.New("fixture read failed")
	runtime := &app.Runtime{KeyringOptions: testKeyringOptions(), Services: app.Services{Docs: func(context.Context, string) (*docs.Service, error) { return nil, want }}}
	doc, err := readLiveVerificationDocument("proof@example.com", "fixture", runtime)
	require.ErrorIs(t, err, want)
	require.Nil(t, doc)
}
