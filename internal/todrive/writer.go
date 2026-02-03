package todrive

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/sheets/v4"

	"github.com/steipete/gogcli/internal/googleapi"
)

var newDriveService = googleapi.NewDrive
var newSheetsService = googleapi.NewSheets

const defaultSheetName = "Report"

// Options control Google Sheets output.
type Options struct {
	SheetName string
	FolderID  string
	Timestamp bool
	Notify    string
	Update    bool
}

// Result describes the created or updated spreadsheet.
type Result struct {
	SpreadsheetID string
	SheetName     string
	URL           string
}

type Writer struct {
	drive  *drive.Service
	sheets *sheets.Service
}

func New(ctx context.Context, account string) (*Writer, error) {
	driveSvc, err := newDriveService(ctx, account)
	if err != nil {
		return nil, err
	}
	sheetsSvc, err := newSheetsService(ctx, account)
	if err != nil {
		return nil, err
	}
	return &Writer{drive: driveSvc, sheets: sheetsSvc}, nil
}

func (w *Writer) Write(ctx context.Context, headers []string, rows [][]string, opts Options) (*Result, error) {
	title := strings.TrimSpace(opts.SheetName)
	if title == "" {
		title = defaultSheetName
	}
	if opts.Timestamp {
		title = fmt.Sprintf("%s-%s", title, time.Now().Format("2006-01-02-150405"))
	}

	spreadsheetID := ""
	spreadsheetURL := ""
	if opts.Update {
		id, url, err := w.findSpreadsheet(ctx, title, opts.FolderID)
		if err != nil {
			return nil, err
		}
		spreadsheetID = id
		spreadsheetURL = url
	}

	if spreadsheetID == "" {
		created, err := w.sheets.Spreadsheets.Create(&sheets.Spreadsheet{
			Properties: &sheets.SpreadsheetProperties{Title: title},
		}).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("create sheet: %w", err)
		}
		spreadsheetID = created.SpreadsheetId
		spreadsheetURL = created.SpreadsheetUrl
		if strings.TrimSpace(opts.FolderID) != "" {
			if err := w.moveToFolder(ctx, spreadsheetID, opts.FolderID); err != nil {
				return nil, err
			}
		}
	}

	if spreadsheetID == "" {
		return nil, fmt.Errorf("missing spreadsheet id")
	}

	if opts.Update {
		_, _ = w.sheets.Spreadsheets.Values.Clear(spreadsheetID, "Sheet1", &sheets.ClearValuesRequest{}).Context(ctx).Do()
	}

	values := make([][]interface{}, 0, len(rows)+1)
	if len(headers) > 0 {
		values = append(values, toInterfaceRow(headers))
	}
	for _, row := range rows {
		values = append(values, toInterfaceRow(row))
	}

	_, err := w.sheets.Spreadsheets.Values.Update(spreadsheetID, "Sheet1!A1", &sheets.ValueRange{
		Values: values,
	}).ValueInputOption("RAW").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("update sheet: %w", err)
	}

	if strings.TrimSpace(opts.Notify) != "" {
		if err := w.shareWith(ctx, spreadsheetID, strings.TrimSpace(opts.Notify)); err != nil {
			return nil, err
		}
	}

	if spreadsheetURL == "" {
		spreadsheetURL = fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", spreadsheetID)
	}

	return &Result{
		SpreadsheetID: spreadsheetID,
		SheetName:     title,
		URL:           spreadsheetURL,
	}, nil
}

func (w *Writer) findSpreadsheet(ctx context.Context, name, folderID string) (string, string, error) {
	query := fmt.Sprintf("mimeType='application/vnd.google-apps.spreadsheet' and name='%s' and trashed=false", escapeDriveQuery(name))
	if strings.TrimSpace(folderID) != "" {
		query = fmt.Sprintf("%s and '%s' in parents", query, strings.TrimSpace(folderID))
	}
	resp, err := w.drive.Files.List().Q(query).Fields("files(id,name,webViewLink)").Context(ctx).Do()
	if err != nil {
		return "", "", fmt.Errorf("find sheet: %w", err)
	}
	if len(resp.Files) == 0 {
		return "", "", nil
	}
	file := resp.Files[0]
	return file.Id, file.WebViewLink, nil
}

func (w *Writer) moveToFolder(ctx context.Context, fileID, folderID string) error {
	file, err := w.drive.Files.Get(fileID).Fields("parents").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("fetch parents: %w", err)
	}
	remove := strings.Join(file.Parents, ",")
	call := w.drive.Files.Update(fileID, nil).AddParents(folderID)
	if remove != "" {
		call = call.RemoveParents(remove)
	}
	if _, err := call.Context(ctx).Do(); err != nil {
		return fmt.Errorf("move sheet: %w", err)
	}
	return nil
}

func (w *Writer) shareWith(ctx context.Context, fileID, email string) error {
	_, err := w.drive.Permissions.Create(fileID, &drive.Permission{
		Type:         "user",
		Role:         "reader",
		EmailAddress: email,
	}).SendNotificationEmail(true).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("share sheet: %w", err)
	}
	return nil
}

func toInterfaceRow(values []string) []interface{} {
	row := make([]interface{}, len(values))
	for i, value := range values {
		row[i] = value
	}
	return row
}

func escapeDriveQuery(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
}
