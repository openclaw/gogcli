package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/drivelabels/v2"
	gapi "google.golang.org/api/googleapi"

	"github.com/openclaw/gogcli/internal/errfmt"
	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type DriveLabelsCmd struct {
	List DriveLabelsListCmd `cmd:"" name:"list" aliases:"ls" help:"List Drive label schemas"`
	Get  DriveLabelsGetCmd  `cmd:"" name:"get" aliases:"info,show" help:"Get a Drive label schema"`
	File DriveLabelsFileCmd `cmd:"" name:"file" help:"List, apply, or remove labels on Drive files"`
}

type DriveLabelsFileCmd struct {
	List   DriveLabelsFileListCmd   `cmd:"" name:"list" aliases:"ls" help:"List labels applied to a Drive file"`
	Apply  DriveLabelsFileApplyCmd  `cmd:"" name:"apply" help:"Apply or update a label on a Drive file"`
	Remove DriveLabelsFileRemoveCmd `cmd:"" name:"remove" aliases:"rm" help:"Remove a label from a Drive file"`
}

type DriveLabelsListCmd struct {
	Max           int64  `name:"max" aliases:"limit" help:"Max results" default:"50"`
	Page          string `name:"page" aliases:"cursor" help:"Page token"`
	Customer      string `name:"customer" help:"Customer resource (for example customers/123abc789); Google Workspace customer required"`
	Language      string `name:"language" help:"BCP-47 language code"`
	View          string `name:"view" help:"Label view: LABEL_VIEW_BASIC|LABEL_VIEW_FULL" default:"LABEL_VIEW_BASIC"`
	MinimumRole   string `name:"minimum-role" help:"Minimum role filter (for example READER, APPLIER, ORGANIZER)"`
	PublishedOnly bool   `name:"published-only" help:"Only list published labels" default:"true" negatable:"_"`
	AdminAccess   bool   `name:"admin-access" help:"Use admin access for Workspace admin accounts"`
	Fields        string `name:"fields" help:"Drive Labels API field mask override"`
}

type DriveLabelsFileListCmd struct {
	FileID string `arg:"" name:"fileId" help:"Drive file ID"`
	Max    int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page   string `name:"page" aliases:"cursor" help:"Page token"`
	Fields string `name:"fields" help:"Drive API field mask override"`
}

func (c *DriveLabelsFileListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := strings.TrimSpace(c.FileID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	call := svc.Files.ListLabels(fileID).MaxResults(c.Max).PageToken(strings.TrimSpace(c.Page)).Context(ctx)
	fields := strings.TrimSpace(c.Fields)
	if fields == "" {
		fields = "labels(id,revisionId,fields),nextPageToken"
	}
	resp, err := call.Fields(gapi.Field(fields)).Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"labels":        resp.Labels,
			"labelCount":    len(resp.Labels),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Labels) == 0 {
		u.Err().Println("No file labels")
		return nil
	}
	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "LABEL_ID\tREVISION\tFIELDS")
	for _, label := range resp.Labels {
		fmt.Fprintf(w, "%s\t%s\t%d\n", label.Id, label.RevisionId, len(label.Fields))
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type DriveLabelsFileApplyCmd struct {
	FileID     string   `arg:"" name:"fileId" help:"Drive file ID"`
	LabelID    string   `arg:"" name:"labelId" help:"Label ID or labels/{id}"`
	Text       []string `name:"text" help:"Set text field as field=value (repeatable)"`
	Selection  []string `name:"selection" help:"Set selection field as field=choiceId[,choiceId] (repeatable)"`
	Integer    []string `name:"integer" help:"Set integer field as field=123[,456] (repeatable)"`
	Date       []string `name:"date" help:"Set date field as field=YYYY-MM-DD[,YYYY-MM-DD] (repeatable)"`
	User       []string `name:"user" help:"Set user field as field=email[,email] (repeatable)"`
	Unset      []string `name:"unset" help:"Unset field ID (repeatable)"`
	FieldsJSON string   `name:"fields-json" help:"Simple JSON object of fieldId to string/number/bool/string-array values (strings become text fields)"`
}

func (c *DriveLabelsFileApplyCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := strings.TrimSpace(c.FileID)
	labelID := normalizeDriveLabelID(c.LabelID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if labelID == "" {
		return usage("empty labelId")
	}
	fieldMods, err := driveLabelFieldMods(c)
	if err != nil {
		return err
	}
	req := &drive.ModifyLabelsRequest{
		LabelModifications: []*drive.LabelModification{{
			LabelId:            labelID,
			FieldModifications: fieldMods,
		}},
	}
	if confirmErr := dryRunAndConfirmDestructive(ctx, flags, "drive.labels.file.apply", req, fmt.Sprintf("apply label %s to drive file %s", labelID, fileID)); confirmErr != nil {
		return confirmErr
	}
	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := svc.Files.ModifyLabels(fileID, req).
		Fields("modifiedLabels(id,revisionId,fields)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"modifiedLabels": resp.ModifiedLabels})
	}
	return writeResult(ctx, u, kv("applied", true), kv("fileId", fileID), kv("labelId", labelID))
}

type DriveLabelsFileRemoveCmd struct {
	FileID  string `arg:"" name:"fileId" help:"Drive file ID"`
	LabelID string `arg:"" name:"labelId" help:"Label ID or labels/{id}"`
}

func (c *DriveLabelsFileRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	fileID := strings.TrimSpace(c.FileID)
	labelID := normalizeDriveLabelID(c.LabelID)
	if fileID == "" {
		return usage("empty fileId")
	}
	if labelID == "" {
		return usage("empty labelId")
	}
	req := &drive.ModifyLabelsRequest{
		LabelModifications: []*drive.LabelModification{{
			LabelId:     labelID,
			RemoveLabel: true,
		}},
	}
	if err := dryRunAndConfirmDestructive(ctx, flags, "drive.labels.file.remove", req, fmt.Sprintf("remove label %s from drive file %s", labelID, fileID)); err != nil {
		return err
	}
	_, svc, err := requireDriveService(ctx, flags)
	if err != nil {
		return err
	}
	resp, err := svc.Files.ModifyLabels(fileID, req).
		Fields("modifiedLabels(id,revisionId,fields)").
		Context(ctx).
		Do()
	if err != nil {
		return err
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"modifiedLabels": resp.ModifiedLabels})
	}
	return writeResult(ctx, u, kv("removed", true), kv("fileId", fileID), kv("labelId", labelID))
}

func (c *DriveLabelsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if c.Max <= 0 {
		return usage("max must be > 0")
	}
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := driveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Labels.List().
		PageSize(c.Max).
		PageToken(strings.TrimSpace(c.Page)).
		View(strings.TrimSpace(c.View)).
		PublishedOnly(c.PublishedOnly).
		UseAdminAccess(c.AdminAccess).
		Context(ctx)
	if strings.TrimSpace(c.Customer) != "" {
		call = call.Customer(strings.TrimSpace(c.Customer))
	}
	if strings.TrimSpace(c.Language) != "" {
		call = call.LanguageCode(strings.TrimSpace(c.Language))
	}
	if strings.TrimSpace(c.MinimumRole) != "" {
		call = call.MinimumRole(strings.TrimSpace(c.MinimumRole))
	}
	fields := strings.TrimSpace(c.Fields)
	if fields == "" {
		fields = "labels(name,id,revisionId,labelType,properties(title,description),lifecycle(state,hasUnpublishedChanges)),nextPageToken"
	}
	resp, err := call.Fields(gapi.Field(fields)).Do()
	if err != nil {
		return wrapDriveLabelsError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"labels":        resp.Labels,
			"labelCount":    len(resp.Labels),
			"nextPageToken": resp.NextPageToken,
		})
	}
	if len(resp.Labels) == 0 {
		u.Err().Println("No labels")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "NAME\tTITLE\tTYPE\tSTATE\tREVISION")
	for _, label := range resp.Labels {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			label.Name,
			driveLabelTitle(label),
			label.LabelType,
			driveLabelState(label),
			label.RevisionId,
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type DriveLabelsGetCmd struct {
	Name        string `arg:"" name:"name" help:"Label name or ID (labels/{id} accepted)"`
	Language    string `name:"language" help:"BCP-47 language code"`
	View        string `name:"view" help:"Label view: LABEL_VIEW_BASIC|LABEL_VIEW_FULL" default:"LABEL_VIEW_FULL"`
	AdminAccess bool   `name:"admin-access" help:"Use admin access for Workspace admin accounts"`
	Fields      string `name:"fields" help:"Drive Labels API field mask override"`
}

func (c *DriveLabelsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	name := normalizeDriveLabelName(c.Name)
	if name == "" {
		return usage("empty label name")
	}
	svc, err := driveLabelsService(ctx, account)
	if err != nil {
		return err
	}

	call := svc.Labels.Get(name).
		View(strings.TrimSpace(c.View)).
		UseAdminAccess(c.AdminAccess).
		Context(ctx)
	if strings.TrimSpace(c.Language) != "" {
		call = call.LanguageCode(strings.TrimSpace(c.Language))
	}
	fields := strings.TrimSpace(c.Fields)
	if fields == "" {
		fields = "name,id,revisionId,labelType,properties(title,description),lifecycle(state,hasUnpublishedChanges),fields"
	}
	label, err := call.Fields(gapi.Field(fields)).Do()
	if err != nil {
		return wrapDriveLabelsError(err)
	}
	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"label": label})
	}

	u.Out().Linef("name\t%s", label.Name)
	u.Out().Linef("title\t%s", driveLabelTitle(label))
	u.Out().Linef("type\t%s", label.LabelType)
	u.Out().Linef("state\t%s", driveLabelState(label))
	u.Out().Linef("revision\t%s", label.RevisionId)
	if label.Properties != nil && strings.TrimSpace(label.Properties.Description) != "" {
		u.Out().Linef("description\t%s", label.Properties.Description)
	}
	if len(label.Fields) > 0 {
		u.Out().Linef("fields\t%d", len(label.Fields))
	}
	return nil
}

func normalizeDriveLabelName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasPrefix(name, "labels/") {
		return name
	}
	return "labels/" + name
}

func normalizeDriveLabelID(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "labels/")
	return strings.TrimSpace(name)
}

func driveLabelFieldMods(c *DriveLabelsFileApplyCmd) ([]*drive.LabelFieldModification, error) {
	mods := make([]*drive.LabelFieldModification, 0)
	add := func(raw []string, setter func(*drive.LabelFieldModification, []string) error) error {
		for _, item := range raw {
			field, values, err := parseDriveLabelFieldAssignment(item)
			if err != nil {
				return err
			}
			mod := &drive.LabelFieldModification{FieldId: field}
			if err := setter(mod, values); err != nil {
				return err
			}
			mods = append(mods, mod)
		}
		return nil
	}
	if err := add(c.Text, func(m *drive.LabelFieldModification, values []string) error {
		m.SetTextValues = values
		return nil
	}); err != nil {
		return nil, err
	}
	if err := add(c.Selection, func(m *drive.LabelFieldModification, values []string) error {
		m.SetSelectionValues = values
		return nil
	}); err != nil {
		return nil, err
	}
	if err := add(c.Integer, func(m *drive.LabelFieldModification, values []string) error {
		ints := make([]int64, 0, len(values))
		for _, value := range values {
			n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
			if err != nil {
				return usagef("invalid integer label value %q: %v", value, err)
			}
			ints = append(ints, n)
		}
		m.SetIntegerValues = ints
		return nil
	}); err != nil {
		return nil, err
	}
	if err := add(c.Date, func(m *drive.LabelFieldModification, values []string) error {
		for _, value := range values {
			if _, err := time.Parse("2006-01-02", strings.TrimSpace(value)); err != nil {
				return usagef("invalid date label value %q (expected YYYY-MM-DD)", value)
			}
		}
		m.SetDateValues = values
		return nil
	}); err != nil {
		return nil, err
	}
	if err := add(c.User, func(m *drive.LabelFieldModification, values []string) error {
		for _, value := range values {
			if err := validatePlainEmail("--user", strings.TrimSpace(value)); err != nil {
				return err
			}
		}
		m.SetUserValues = values
		return nil
	}); err != nil {
		return nil, err
	}
	for _, field := range c.Unset {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, usage("empty --unset field")
		}
		mods = append(mods, &drive.LabelFieldModification{FieldId: field, UnsetValues: true})
	}
	if strings.TrimSpace(c.FieldsJSON) != "" {
		jsonMods, err := parseDriveLabelFieldsJSON(c.FieldsJSON)
		if err != nil {
			return nil, err
		}
		mods = append(mods, jsonMods...)
	}
	return mods, nil
}

func parseDriveLabelFieldAssignment(raw string) (string, []string, error) {
	field, value, ok := strings.Cut(strings.TrimSpace(raw), "=")
	if !ok || strings.TrimSpace(field) == "" {
		return "", nil, usage("label field assignments must be field=value")
	}
	values := splitCSV(value)
	if len(values) == 0 {
		values = []string{""}
	}
	return strings.TrimSpace(field), values, nil
}

func parseDriveLabelFieldsJSON(raw string) ([]*drive.LabelFieldModification, error) {
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		return nil, usagef("parse --fields-json: %v", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, usagef("parse --fields-json: %v", err)
		}
		return nil, usage("parse --fields-json: trailing JSON value")
	}
	mods := make([]*drive.LabelFieldModification, 0, len(obj))
	for field, value := range obj {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, usage("empty field key in --fields-json")
		}
		mod := &drive.LabelFieldModification{FieldId: field}
		switch v := value.(type) {
		case string:
			mod.SetTextValues = []string{v}
		case json.Number:
			n, err := v.Int64()
			if err != nil {
				return nil, usagef("invalid integer label value %q in --fields-json", v.String())
			}
			mod.SetIntegerValues = []int64{n}
		case bool:
			mod.SetTextValues = []string{strconv.FormatBool(v)}
		case []any:
			values := make([]string, 0, len(v))
			for _, item := range v {
				values = append(values, fmt.Sprint(item))
			}
			mod.SetTextValues = values
		case nil:
			mod.UnsetValues = true
		default:
			return nil, usagef("unsupported --fields-json value for %s", field)
		}
		mods = append(mods, mod)
	}
	return mods, nil
}

func driveLabelTitle(label *drivelabels.GoogleAppsDriveLabelsV2Label) string {
	if label == nil || label.Properties == nil {
		return ""
	}
	return label.Properties.Title
}

func driveLabelState(label *drivelabels.GoogleAppsDriveLabelsV2Label) string {
	if label == nil || label.Lifecycle == nil {
		return ""
	}
	return label.Lifecycle.State
}

func wrapDriveLabelsError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "without a valid customer") {
		return errfmt.NewUserFacingError("Drive Labels API requires a Google Workspace customer; consumer Google accounts may not have a valid customer.", err)
	}
	return err
}
