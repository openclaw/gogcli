package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	"google.golang.org/api/docs/v1"
	gapi "google.golang.org/api/googleapi"

	"github.com/steipete/gogcli/internal/config"
)

func resolveContentInput(content, filePath string) (string, error) {
	if content != "" {
		return content, nil
	}
	if filePath != "" {
		if filePath == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", fmt.Errorf("reading stdin: %w", err)
			}
			return string(data), nil
		}
		data, err := os.ReadFile(filePath) //nolint:gosec // user-provided path
		if err != nil {
			return "", fmt.Errorf("reading file: %w", err)
		}
		return string(data), nil
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	return "", nil
}

func docsWebViewLink(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "https://docs.google.com/document/d/" + id + "/edit"
}

func setDocumentPageless(ctx context.Context, svc *docs.Service, docID string) error {
	_, err := svc.Documents.BatchUpdate(docID, &docs.BatchUpdateDocumentRequest{
		Requests: []*docs.Request{{
			UpdateDocumentStyle: &docs.UpdateDocumentStyleRequest{
				DocumentStyle: &docs.DocumentStyle{
					DocumentFormat: &docs.DocumentFormat{DocumentMode: "PAGELESS"},
				},
				Fields: "documentFormat",
			},
		}},
	}).Context(ctx).Do()
	return err
}

func resolveTextInput(text, file string, kctx *kong.Context, textFlag, fileFlag string) (string, bool, error) {
	file = strings.TrimSpace(file)
	textProvided := text != "" || flagProvided(kctx, textFlag)
	fileProvided := file != "" || flagProvided(kctx, fileFlag)
	if textProvided && fileProvided {
		return "", true, usage(fmt.Sprintf("use only one of --%s or --%s", textFlag, fileFlag))
	}
	if fileProvided {
		b, err := readTextInput(file)
		if err != nil {
			return "", true, err
		}
		return string(b), true, nil
	}
	if textProvided {
		return text, true, nil
	}
	return text, false, nil
}

func readTextInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	expanded, err := config.ExpandPath(path)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(expanded) //nolint:gosec // user-provided path
}

func docsDocumentEndIndex(doc *docs.Document) int64 {
	if doc == nil || doc.Body == nil {
		return 1
	}
	end := int64(1)
	for _, el := range doc.Body.Content {
		if el == nil {
			continue
		}
		if el.EndIndex > end {
			end = el.EndIndex
		}
	}
	return end
}

func docsTabEndIndex(tab *docs.Tab) int64 {
	if tab == nil || tab.DocumentTab == nil || tab.DocumentTab.Body == nil {
		return 1
	}
	end := int64(1)
	for _, el := range tab.DocumentTab.Body.Content {
		if el == nil {
			continue
		}
		if el.EndIndex > end {
			end = el.EndIndex
		}
	}
	return end
}

func docsTargetEndIndexAndTabID(ctx context.Context, svc *docs.Service, docID, tabQuery string) (int64, string, error) {
	getCall := svc.Documents.Get(docID).Context(ctx)
	if tabQuery != "" {
		getCall = getCall.IncludeTabsContent(true)
	} else {
		getCall = getCall.Fields("documentId,body/content(startIndex,endIndex)")
	}

	doc, err := getCall.Do()
	if err != nil {
		if isDocsNotFound(err) {
			return 0, "", fmt.Errorf("doc not found or not a Google Doc (id=%s)", docID)
		}
		return 0, "", err
	}
	if doc == nil {
		return 0, "", errors.New("doc not found")
	}
	if tabQuery == "" {
		return docsDocumentEndIndex(doc), "", nil
	}

	tab, tabErr := findTab(flattenTabs(doc.Tabs), tabQuery)
	if tabErr != nil {
		return 0, "", tabErr
	}
	if tab.TabProperties == nil || strings.TrimSpace(tab.TabProperties.TabId) == "" {
		return 0, "", fmt.Errorf("tab has no ID: %s", tabQuery)
	}
	return docsTabEndIndex(tab), tab.TabProperties.TabId, nil
}

func resolveDocsTabID(ctx context.Context, svc *docs.Service, docID, tabQuery string) (string, error) {
	_, tabID, err := docsTargetEndIndexAndTabID(ctx, svc, docID, tabQuery)
	return tabID, err
}

func docsAppendIndex(endIndex int64) int64 {
	if endIndex > 1 {
		return endIndex - 1
	}
	return 1
}

func isDocsNotFound(err error) bool {
	var apiErr *gapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusNotFound
}
