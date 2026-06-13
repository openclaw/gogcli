package cmd

import (
	"context"
	"fmt"
	"strings"

	formsapi "google.golang.org/api/forms/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// FormsAddQuestionCmd adds a question to an existing form via batchUpdate.
type FormsAddQuestionCmd struct {
	FormID   string   `arg:"" name:"formId" help:"Form ID"`
	Title    string   `name:"title" help:"Question title/text" required:""`
	Type     string   `name:"type" help:"Question type: text|paragraph|radio|checkbox|dropdown|scale|date|time" default:"text"`
	Required bool     `name:"required" help:"Whether an answer is required"`
	Options  []string `name:"option" help:"Choice options (for radio/checkbox/dropdown, repeat for each)" short:"o"`
	Index    int      `name:"index" help:"Position to insert (0-based, default append)" default:"-1"`
	Correct  []string `name:"correct" help:"Correct answer value for quiz grading (repeat for multiple accepted/checkbox answers)"`
	Points   int      `name:"points" help:"Positive quiz points for the question when --correct is set"`

	// Scale-specific
	ScaleLow       int    `name:"scale-low" help:"Scale minimum value: 0 or 1" default:"1"`
	ScaleHigh      int    `name:"scale-high" help:"Scale maximum value: 2 through 10" default:"5"`
	ScaleLowLabel  string `name:"scale-low-label" help:"Label for low end of scale"`
	ScaleHighLabel string `name:"scale-high-label" help:"Label for high end of scale"`

	// Date/time specific
	IncludeTime bool `name:"include-time" help:"Include time picker (for date type)"`
	IncludeYear bool `name:"include-year" help:"Include year field (for date type)"`
	Duration    bool `name:"duration" help:"Ask for duration instead of time (for time type)"`

	Description string `name:"description" help:"Question description/help text"`
}

func (c *FormsAddQuestionCmd) Run(ctx context.Context, flags *RootFlags) error {
	plan, err := newFormsAddQuestionPlan(formsAddQuestionInput{
		FormID:         c.FormID,
		Title:          c.Title,
		Type:           c.Type,
		Required:       c.Required,
		Options:        c.Options,
		Index:          c.Index,
		Correct:        c.Correct,
		Points:         c.Points,
		ScaleLow:       c.ScaleLow,
		ScaleHigh:      c.ScaleHigh,
		ScaleLowLabel:  c.ScaleLowLabel,
		ScaleHighLabel: c.ScaleHighLabel,
		IncludeTime:    c.IncludeTime,
		IncludeYear:    c.IncludeYear,
		Duration:       c.Duration,
		Description:    c.Description,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.add-question", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := formsService(ctx, account)
	if err != nil {
		return err
	}

	currentItemCount := 0
	if plan.needsCurrentForm() {
		currentForm, getErr := svc.Forms.Get(plan.FormID).Context(ctx).Do()
		if getErr != nil {
			return getErr
		}
		currentItemCount = len(currentForm.Items)
	}

	batchReq := plan.batchRequest(currentItemCount)
	resp, err := svc.Forms.BatchUpdate(plan.FormID, batchReq).Context(ctx).Do()
	if err != nil {
		return err
	}

	insertIndex := batchReq.Requests[0].CreateItem.Location.Index

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"created":  true,
			"form_id":  plan.FormID,
			"title":    plan.Title,
			"type":     plan.Type,
			"index":    insertIndex,
			"form":     resp.Form,
			"edit_url": formEditURL(plan.FormID),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("created\ttrue")
	u.Out().Linef("form_id\t%s", plan.FormID)
	u.Out().Linef("question\t%s", plan.Title)
	u.Out().Linef("type\t%s", plan.Type)
	u.Out().Linef("index\t%d", insertIndex)
	u.Out().Linef("edit_url\t%s", formEditURL(plan.FormID))
	return nil
}

func buildQuestion(qType string, input *formsAddQuestionInput) (*formsapi.Question, error) {
	q := &formsapi.Question{
		Required: input.Required,
	}

	switch qType {
	case "text":
		q.TextQuestion = &formsapi.TextQuestion{Paragraph: false}
	case "paragraph":
		q.TextQuestion = &formsapi.TextQuestion{Paragraph: true}
	case "radio", "checkbox", "dropdown":
		if len(input.Options) == 0 {
			return nil, usage("--option is required for " + qType + " questions (repeat for each choice)")
		}
		apiType := map[string]string{
			"radio":    "RADIO",
			"checkbox": "CHECKBOX",
			"dropdown": "DROP_DOWN",
		}[qType]
		opts := make([]*formsapi.Option, len(input.Options))
		for i, v := range input.Options {
			opts[i] = &formsapi.Option{Value: v}
		}
		q.ChoiceQuestion = &formsapi.ChoiceQuestion{
			Type:    apiType,
			Options: opts,
		}
	case "scale":
		if input.ScaleLow != 0 && input.ScaleLow != 1 {
			return nil, usage("--scale-low must be 0 or 1")
		}
		if input.ScaleHigh < 2 || input.ScaleHigh > 10 {
			return nil, usage("--scale-high must be between 2 and 10")
		}
		q.ScaleQuestion = &formsapi.ScaleQuestion{
			Low:       int64(input.ScaleLow),
			High:      int64(input.ScaleHigh),
			LowLabel:  input.ScaleLowLabel,
			HighLabel: input.ScaleHighLabel,
		}
	case "date":
		q.DateQuestion = &formsapi.DateQuestion{
			IncludeTime: input.IncludeTime,
			IncludeYear: input.IncludeYear,
		}
	case "time":
		q.TimeQuestion = &formsapi.TimeQuestion{
			Duration: input.Duration,
		}
	default:
		return nil, usage("unknown question type: " + qType + " (use text|paragraph|radio|checkbox|dropdown|scale|date|time)")
	}

	if err := applyQuestionGrading(q, qType, input); err != nil {
		return nil, err
	}

	return q, nil
}

func applyQuestionGrading(q *formsapi.Question, qType string, input *formsAddQuestionInput) error {
	correct := cleanedStrings(input.Correct)
	hasCorrect := len(correct) > 0
	hasPoints := input.Points > 0
	if !hasCorrect && !hasPoints {
		return nil
	}
	if !hasCorrect {
		return usage("--points requires at least one --correct answer")
	}
	if !hasPoints {
		return usage("--correct requires --points")
	}
	switch qType {
	case "text", "radio", "checkbox", "dropdown":
	default:
		return usage("--correct is supported only for text, radio, checkbox, and dropdown questions")
	}

	answers := make([]*formsapi.CorrectAnswer, len(correct))
	for i, value := range correct {
		answers[i] = &formsapi.CorrectAnswer{Value: value}
	}
	q.Grading = &formsapi.Grading{
		PointValue:     int64(input.Points),
		CorrectAnswers: &formsapi.CorrectAnswers{Answers: answers},
		ForceSendFields: []string{
			"PointValue",
		},
	}
	return nil
}

func cleanedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// FormsDeleteQuestionCmd removes a question from a form by index.
type FormsDeleteQuestionCmd struct {
	FormID string `arg:"" name:"formId" help:"Form ID"`
	Index  int    `arg:"" name:"index" help:"Question index (0-based)"`
}

func (c *FormsDeleteQuestionCmd) Run(ctx context.Context, flags *RootFlags) error {
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	if c.Index < 0 {
		return usage("index must be >= 0")
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.delete-question", map[string]any{
		"form_id": formID,
		"index":   c.Index,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := formsService(ctx, account)
	if err != nil {
		return err
	}

	form, err := svc.Forms.Get(formID).Context(ctx).Do()
	if err != nil {
		return err
	}
	if c.Index >= len(form.Items) {
		return usagef("question index %d out of range (form has %d items)", c.Index, len(form.Items))
	}

	if confirmErr := confirmDestructiveChecked(ctx, flagsWithoutDryRun(flags), fmt.Sprintf("delete question %d from form %s", c.Index, formID)); confirmErr != nil {
		return confirmErr
	}

	batchReq := &formsapi.BatchUpdateFormRequest{
		Requests: []*formsapi.Request{
			{
				DeleteItem: &formsapi.DeleteItemRequest{
					Location: formLocationIndex(c.Index),
				},
			},
		},
	}

	_, err = svc.Forms.BatchUpdate(formID, batchReq).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"deleted": true,
			"form_id": formID,
			"index":   c.Index,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("deleted\ttrue")
	u.Out().Linef("form_id\t%s", formID)
	u.Out().Linef("index\t%d", c.Index)
	return nil
}

// FormsMoveQuestionCmd moves a question to a new position.
type FormsMoveQuestionCmd struct {
	FormID   string `arg:"" name:"formId" help:"Form ID"`
	OldIndex int    `arg:"" name:"oldIndex" help:"Current question index (0-based)"`
	NewIndex int    `arg:"" name:"newIndex" help:"Target question index (0-based)"`
}

func (c *FormsMoveQuestionCmd) Run(ctx context.Context, flags *RootFlags) error {
	formID := strings.TrimSpace(normalizeGoogleID(c.FormID))
	if formID == "" {
		return usage("empty formId")
	}
	if c.OldIndex < 0 || c.NewIndex < 0 {
		return usage("indices must be >= 0")
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.move-question", map[string]any{
		"form_id":   formID,
		"old_index": c.OldIndex,
		"new_index": c.NewIndex,
	}); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := formsService(ctx, account)
	if err != nil {
		return err
	}

	batchReq := &formsapi.BatchUpdateFormRequest{
		Requests: []*formsapi.Request{
			{
				MoveItem: &formsapi.MoveItemRequest{
					OriginalLocation: formLocationIndex(c.OldIndex),
					NewLocation:      formLocationIndex(c.NewIndex),
				},
			},
		},
	}

	_, err = svc.Forms.BatchUpdate(formID, batchReq).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"moved":     true,
			"form_id":   formID,
			"old_index": c.OldIndex,
			"new_index": c.NewIndex,
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("moved\ttrue")
	u.Out().Linef("form_id\t%s", formID)
	u.Out().Linef("old_index\t%d", c.OldIndex)
	u.Out().Linef("new_index\t%d", c.NewIndex)
	return nil
}

func formLocationIndex(index int) *formsapi.Location {
	return &formsapi.Location{
		Index:           int64(index),
		ForceSendFields: []string{"Index"},
	}
}

// FormsUpdateCmd modifies form title, description, or settings.
type FormsUpdateCmd struct {
	FormID      string `arg:"" name:"formId" help:"Form ID"`
	Title       string `name:"title" help:"New form title"`
	Description string `name:"description" help:"New form description"`
	IsQuiz      string `name:"quiz" help:"Enable quiz mode (true/false)"`
}

func (c *FormsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	plan, err := newFormsUpdatePlan(formsUpdateInput{
		FormID:      c.FormID,
		Title:       c.Title,
		Description: c.Description,
		Quiz:        c.IsQuiz,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "forms.update", plan.dryRunPayload()); dryRunErr != nil {
		return dryRunErr
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	svc, err := formsService(ctx, account)
	if err != nil {
		return err
	}

	resp, err := svc.Forms.BatchUpdate(plan.FormID, plan.Request).Context(ctx).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			"updated":  true,
			"form_id":  plan.FormID,
			"form":     resp.Form,
			"edit_url": formEditURL(plan.FormID),
		})
	}

	u := ui.FromContext(ctx)
	u.Out().Linef("updated\ttrue")
	printFormSummary(u, resp.Form, plan.FormID)
	return nil
}
