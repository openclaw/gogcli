package cmd

import (
	"context"
	"fmt"
	"sort"

	"google.golang.org/api/docs/v1"
	"google.golang.org/api/drive/v3"
	formsapi "google.golang.org/api/forms/v1"
	gapi "google.golang.org/api/googleapi"
	scriptapi "google.golang.org/api/script/v1"
	"google.golang.org/api/sheets/v4"
	"google.golang.org/api/slides/v1"

	"github.com/steipete/gogcli/internal/backup"
)

const (
	driveMimeGoogleForm   = "application/vnd.google-apps.form"
	driveMimeGoogleScript = "application/vnd.google-apps.script"
)

type workspaceBackupDoc struct {
	File     *drive.File    `json:"file"`
	Document *docs.Document `json:"document,omitempty"`
	Error    string         `json:"error,omitempty"`
}

type workspaceBackupSheet struct {
	File        *drive.File         `json:"file"`
	Spreadsheet *sheets.Spreadsheet `json:"spreadsheet,omitempty"`
	Error       string              `json:"error,omitempty"`
}

type workspaceBackupSlides struct {
	File         *drive.File          `json:"file"`
	Presentation *slides.Presentation `json:"presentation,omitempty"`
	Error        string               `json:"error,omitempty"`
}

type formsBackupForm struct {
	File      *drive.File              `json:"file"`
	Form      *formsapi.Form           `json:"form,omitempty"`
	Responses []*formsapi.FormResponse `json:"responses,omitempty"`
	Error     string                   `json:"error,omitempty"`
}

type appScriptBackupProject struct {
	File    *drive.File        `json:"file"`
	Project *scriptapi.Project `json:"project,omitempty"`
	Content *scriptapi.Content `json:"content,omitempty"`
	Error   string             `json:"error,omitempty"`
}

type workspaceBackupOptions struct {
	ShardMaxRows int
	Native       bool
	MaxFiles     int
}

func buildWorkspaceBackupSnapshot(ctx context.Context, flags *RootFlags, opts workspaceBackupOptions) (backup.Snapshot, error) {
	account, driveSvc, err := requireDriveService(ctx, flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	docsRows, sheetsRows, slidesRows, err := fetchWorkspaceNativeRows(ctx, flags, driveSvc, opts)
	if err != nil {
		return backup.Snapshot{}, err
	}
	formsRows, formResponses, formsErr := fetchFormsBackupRows(ctx, account, driveSvc)
	if formsErr != nil {
		formsRows = []formsBackupForm{{Error: formsErr.Error()}}
	}
	var shards []backup.PlainShard
	for _, part := range []struct {
		kind string
		rows any
	}{
		{"docs", docsRows},
		{"sheets", sheetsRows},
		{"slides", slidesRows},
		{"forms", formsRows},
	} {
		partShards, shardErr := buildBackupShardsAny(backupServiceWorkspace, part.kind, accountHash, fmt.Sprintf("data/workspace/%s/%s", accountHash, part.kind), part.rows, opts.ShardMaxRows)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		shards = append(shards, partShards...)
	}
	return backup.Snapshot{
		Services: []string{backupServiceWorkspace},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"workspace.docs":            len(docsRows),
			"workspace.sheets":          len(sheetsRows),
			"workspace.slides":          len(slidesRows),
			"workspace.forms":           len(formsRows),
			"workspace.forms.responses": formResponses,
		},
		Shards: shards,
	}, nil
}

func buildAppScriptBackupSnapshot(ctx context.Context, flags *RootFlags, shardMaxRows int) (backup.Snapshot, error) {
	account, driveSvc, err := requireDriveService(ctx, flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	scriptSvc, err := newAppScriptService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	files, err := fetchDriveFilesByMime(ctx, driveSvc, driveMimeGoogleScript)
	if err != nil {
		return backup.Snapshot{}, err
	}
	rows := make([]appScriptBackupProject, 0, len(files))
	for _, file := range files {
		row := appScriptBackupProject{File: file}
		if project, getErr := scriptSvc.Projects.Get(file.Id).Context(ctx).Do(); getErr == nil {
			row.Project = project
		} else {
			row.Error = getErr.Error()
		}
		if row.Error == "" {
			if content, contentErr := scriptSvc.Projects.GetContent(file.Id).Context(ctx).Do(); contentErr == nil {
				row.Content = content
			} else {
				row.Error = contentErr.Error()
			}
		}
		rows = append(rows, row)
	}
	shards, err := buildBackupShards(backupServiceAppScript, "projects", accountHash, fmt.Sprintf("data/appscript/%s/projects", accountHash), rows, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	return backup.Snapshot{
		Services: []string{backupServiceAppScript},
		Accounts: []string{accountHash},
		Counts:   map[string]int{"appscript.projects": len(rows)},
		Shards:   shards,
	}, nil
}

func fetchFormsBackupRows(ctx context.Context, account string, driveSvc *drive.Service) ([]formsBackupForm, int, error) {
	formsSvc, err := newFormsService(ctx, account)
	if err != nil {
		return nil, 0, err
	}
	files, err := fetchDriveFilesByMime(ctx, driveSvc, driveMimeGoogleForm)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]formsBackupForm, 0, len(files))
	responseCount := 0
	for _, file := range files {
		row := formsBackupForm{File: file}
		if form, getErr := formsSvc.Forms.Get(file.Id).Context(ctx).Do(); getErr == nil {
			row.Form = form
		} else {
			row.Error = getErr.Error()
		}
		if row.Error == "" {
			responses, responsesErr := fetchBackupFormResponses(ctx, formsSvc, file.Id)
			if responsesErr == nil {
				row.Responses = responses
				responseCount += len(responses)
			} else {
				row.Error = responsesErr.Error()
			}
		}
		rows = append(rows, row)
	}
	return rows, responseCount, nil
}

func fetchWorkspaceNativeRows(ctx context.Context, flags *RootFlags, driveSvc *drive.Service, opts workspaceBackupOptions) ([]workspaceBackupDoc, []workspaceBackupSheet, []workspaceBackupSlides, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return nil, nil, nil, err
	}
	docFiles, err := fetchDriveFilesByMime(ctx, driveSvc, driveMimeGoogleDoc)
	if err != nil {
		return nil, nil, nil, err
	}
	sheetFiles, err := fetchDriveFilesByMime(ctx, driveSvc, driveMimeGoogleSheet)
	if err != nil {
		return nil, nil, nil, err
	}
	slideFiles, err := fetchDriveFilesByMime(ctx, driveSvc, driveMimeGoogleSlides)
	if err != nil {
		return nil, nil, nil, err
	}
	docFiles = capDriveFiles(docFiles, opts.MaxFiles)
	sheetFiles = capDriveFiles(sheetFiles, opts.MaxFiles)
	slideFiles = capDriveFiles(slideFiles, opts.MaxFiles)
	var docsSvc *docs.Service
	var sheetsSvc *sheets.Service
	var slidesSvc *slides.Service
	if opts.Native {
		docsSvc, _ = newDocsService(ctx, account)
		sheetsSvc, _ = newSheetsService(ctx, account)
		slidesSvc, _ = newSlidesService(ctx, account)
	}
	docRows := make([]workspaceBackupDoc, 0, len(docFiles))
	for _, file := range docFiles {
		row := workspaceBackupDoc{File: file}
		if !opts.Native {
			row.Error = "native fetch disabled; use --workspace-native"
		} else if docsSvc == nil {
			row.Error = "docs service unavailable"
		} else if doc, getErr := docsSvc.Documents.Get(file.Id).IncludeTabsContent(true).Context(ctx).Do(); getErr == nil {
			row.Document = doc
		} else {
			row.Error = getErr.Error()
		}
		docRows = append(docRows, row)
	}
	sheetRows := make([]workspaceBackupSheet, 0, len(sheetFiles))
	for _, file := range sheetFiles {
		row := workspaceBackupSheet{File: file}
		if !opts.Native {
			row.Error = "native fetch disabled; use --workspace-native"
		} else if sheetsSvc == nil {
			row.Error = "sheets service unavailable"
		} else if sheet, getErr := sheetsSvc.Spreadsheets.Get(file.Id).IncludeGridData(true).Context(ctx).Do(); getErr == nil {
			row.Spreadsheet = sheet
		} else {
			row.Error = getErr.Error()
		}
		sheetRows = append(sheetRows, row)
	}
	slideRows := make([]workspaceBackupSlides, 0, len(slideFiles))
	for _, file := range slideFiles {
		row := workspaceBackupSlides{File: file}
		if !opts.Native {
			row.Error = "native fetch disabled; use --workspace-native"
		} else if slidesSvc == nil {
			row.Error = "slides service unavailable"
		} else if presentation, getErr := slidesSvc.Presentations.Get(file.Id).Context(ctx).Do(); getErr == nil {
			row.Presentation = presentation
		} else {
			row.Error = getErr.Error()
		}
		slideRows = append(slideRows, row)
	}
	return docRows, sheetRows, slideRows, nil
}

func capDriveFiles(files []*drive.File, maxFiles int) []*drive.File {
	if maxFiles <= 0 || len(files) <= maxFiles {
		return files
	}
	return files[:maxFiles]
}

func fetchBackupFormResponses(ctx context.Context, svc *formsapi.Service, formID string) ([]*formsapi.FormResponse, error) {
	var out []*formsapi.FormResponse
	pageToken := ""
	for {
		call := svc.Forms.Responses.List(formID).PageSize(5000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Responses...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

func fetchDriveFilesByMime(ctx context.Context, svc *drive.Service, mimeType string) ([]*drive.File, error) {
	var out []*drive.File
	pageToken := ""
	for {
		call := svc.Files.List().
			Q(fmt.Sprintf("mimeType = '%s' and trashed = false", mimeType)).
			PageSize(1000).
			OrderBy("modifiedTime desc").
			Fields(gapi.Field("nextPageToken, files(id, name, mimeType, size, createdTime, modifiedTime, parents, owners, lastModifyingUser, webViewLink, driveId, md5Checksum, fileExtension)")).
			Context(ctx)
		call = driveFilesListCallWithDriveSupport(call, true, "")
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, err
		}
		out = append(out, resp.Files...)
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out, nil
}

func buildBackupShardsAny(service, kind, accountHash, prefix string, rows any, shardMaxRows int) ([]backup.PlainShard, error) {
	switch typed := rows.(type) {
	case []classroomBackupTopic:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []classroomBackupAnnouncement:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []classroomBackupCourseWork:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []classroomBackupMaterial:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []classroomBackupSubmission:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []workspaceBackupDoc:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []workspaceBackupSheet:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []workspaceBackupSlides:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	case []formsBackupForm:
		return buildBackupShards(service, kind, accountHash, prefix, typed, shardMaxRows)
	default:
		return nil, fmt.Errorf("unsupported backup row type for %s.%s", service, kind)
	}
}
