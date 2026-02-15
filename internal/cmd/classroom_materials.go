package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

type ClassroomMaterialsCmd struct {
	List   ClassroomMaterialsListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List coursework materials"`
	Get    ClassroomMaterialsGetCmd    `cmd:"" aliases:"info,show" help:"Get coursework material"`
	Create ClassroomMaterialsCreateCmd `cmd:"" aliases:"add,new" help:"Create coursework material"`
	Update ClassroomMaterialsUpdateCmd `cmd:"" aliases:"edit,set" help:"Update coursework material"`
	Delete ClassroomMaterialsDeleteCmd `cmd:"" aliases:"rm,del,remove" help:"Delete coursework material"`
}

type ClassroomMaterialsListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	States    string `name:"state" help:"Material states filter (comma-separated: PUBLISHED,DRAFT,DELETED)"`
	Topic     string `name:"topic" help:"Filter by topic ID"`
	OrderBy   string `name:"order-by" help:"Order by (e.g., updateTime desc)"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
	ScanPages int    `name:"scan-pages" help:"Pages to scan when filtering by topic" default:"3"`
}

func (c *ClassroomMaterialsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	makeCall := func(page string) (*classroom.ListCourseWorkMaterialResponse, error) {
		call := svc.Courses.CourseWorkMaterials.List(courseID).PageSize(c.Max).PageToken(page).Context(ctx)
		if states := splitCSV(c.States); len(states) > 0 {
			upper := make([]string, 0, len(states))
			for _, state := range states {
				upper = append(upper, strings.ToUpper(state))
			}
			call.CourseWorkMaterialStates(upper...)
		}
		if v := strings.TrimSpace(c.OrderBy); v != "" {
			call.OrderBy(v)
		}
		return call.Do()
	}

	fetch := func(page string) ([]*classroom.CourseWorkMaterial, string, error) {
		resp, callErr := makeCall(page)
		if callErr != nil {
			return nil, "", callErr
		}
		return resp.CourseWorkMaterial, resp.NextPageToken, nil
	}

	var materials []*classroom.CourseWorkMaterial
	var nextPageToken string
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return wrapClassroomError(err)
		}
		materials = all
		if topic := strings.TrimSpace(c.Topic); topic != "" {
			filtered := materials[:0]
			for _, material := range materials {
				if material == nil {
					continue
				}
				if material.TopicId == topic {
					filtered = append(filtered, material)
				}
			}
			materials = filtered
		}
	} else {
		var err error
		materials, nextPageToken, err = scanClassroomTopicPages(
			c.Topic,
			c.Page,
			c.ScanPages,
			fetch,
			func(material *classroom.CourseWorkMaterial) string {
				if material == nil {
					return ""
				}
				return material.TopicId
			},
		)
		if err != nil {
			return wrapClassroomError(err)
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"materials":     materials,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(materials) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(materials) == 0 {
		u.Err().Println("No materials")
		printNextPageHint(u, nextPageToken)
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ID\tTITLE\tSTATE\tUPDATED")
	for _, material := range materials {
		if material == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			sanitizeTab(material.Id),
			sanitizeTab(material.Title),
			sanitizeTab(material.State),
			sanitizeTab(material.UpdateTime),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ClassroomMaterialsGetCmd struct {
	CourseID   string `arg:"" name:"courseId" help:"Course ID or alias"`
	MaterialID string `arg:"" name:"materialId" help:"Material ID"`
}

func (c *ClassroomMaterialsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	materialID := strings.TrimSpace(c.MaterialID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if materialID == "" {
		return usage("empty materialId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	material, err := svc.Courses.CourseWorkMaterials.Get(courseID, materialID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"material": material})
	}

	u.Out().Printf("id\t%s", material.Id)
	u.Out().Printf("title\t%s", material.Title)
	if material.Description != "" {
		u.Out().Printf("description\t%s", material.Description)
	}
	u.Out().Printf("state\t%s", material.State)
	if material.TopicId != "" {
		u.Out().Printf("topic_id\t%s", material.TopicId)
	}
	if material.ScheduledTime != "" {
		u.Out().Printf("scheduled\t%s", material.ScheduledTime)
	}
	return nil
}

type ClassroomMaterialsCreateCmd struct {
	CourseID    string `arg:"" name:"courseId" help:"Course ID or alias"`
	Title       string `name:"title" help:"Title" required:""`
	Description string `name:"description" help:"Description"`
	State       string `name:"state" help:"State: PUBLISHED, DRAFT"`
	Scheduled   string `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
	TopicID     string `name:"topic" help:"Topic ID"`
}

func (c *ClassroomMaterialsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if strings.TrimSpace(c.Title) == "" {
		return usage("empty title")
	}

	material := &classroom.CourseWorkMaterial{Title: strings.TrimSpace(c.Title)}
	if v := strings.TrimSpace(c.Description); v != "" {
		material.Description = v
	}
	if v := strings.TrimSpace(c.State); v != "" {
		material.State = strings.ToUpper(v)
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		material.ScheduledTime = v
	}
	if v := strings.TrimSpace(c.TopicID); v != "" {
		material.TopicId = v
	}

	if err := dryRunExit(ctx, flags, "classroom.materials.create", map[string]any{
		"course_id": courseID,
		"material":  material,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.CourseWorkMaterials.Create(courseID, material).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"material": created})
	}
	u.Out().Printf("id\t%s", created.Id)
	u.Out().Printf("title\t%s", created.Title)
	u.Out().Printf("state\t%s", created.State)
	return nil
}

type ClassroomMaterialsUpdateCmd struct {
	CourseID    string `arg:"" name:"courseId" help:"Course ID or alias"`
	MaterialID  string `arg:"" name:"materialId" help:"Material ID"`
	Title       string `name:"title" help:"Title"`
	Description string `name:"description" help:"Description"`
	State       string `name:"state" help:"State: PUBLISHED, DRAFT"`
	Scheduled   string `name:"scheduled" help:"Scheduled publish time (RFC3339)"`
	TopicID     string `name:"topic" help:"Topic ID"`
}

func (c *ClassroomMaterialsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	materialID := strings.TrimSpace(c.MaterialID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if materialID == "" {
		return usage("empty materialId")
	}

	material := &classroom.CourseWorkMaterial{}
	fields := make([]string, 0, 4)
	if v := strings.TrimSpace(c.Title); v != "" {
		material.Title = v
		fields = append(fields, "title")
	}
	if v := strings.TrimSpace(c.Description); v != "" {
		material.Description = v
		fields = append(fields, "description")
	}
	if v := strings.TrimSpace(c.State); v != "" {
		material.State = strings.ToUpper(v)
		fields = append(fields, "state")
	}
	if v := strings.TrimSpace(c.Scheduled); v != "" {
		material.ScheduledTime = v
		fields = append(fields, "scheduledTime")
	}
	if v := strings.TrimSpace(c.TopicID); v != "" {
		material.TopicId = v
		fields = append(fields, "topicId")
	}
	if len(fields) == 0 {
		return usage("no updates specified")
	}

	if err := dryRunExit(ctx, flags, "classroom.materials.update", map[string]any{
		"course_id":     courseID,
		"material_id":   materialID,
		"update_mask":   updateMask(fields),
		"update_fields": fields,
		"material":      material,
	}); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.CourseWorkMaterials.Patch(courseID, materialID, material).UpdateMask(updateMask(fields)).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"material": updated})
	}
	u.Out().Printf("id\t%s", updated.Id)
	u.Out().Printf("title\t%s", updated.Title)
	u.Out().Printf("state\t%s", updated.State)
	return nil
}

type ClassroomMaterialsDeleteCmd struct {
	CourseID   string `arg:"" name:"courseId" help:"Course ID or alias"`
	MaterialID string `arg:"" name:"materialId" help:"Material ID"`
}

func (c *ClassroomMaterialsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	materialID := strings.TrimSpace(c.MaterialID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if materialID == "" {
		return usage("empty materialId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete material %s from %s", materialID, courseID)); err != nil {
		return err
	}

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Courses.CourseWorkMaterials.Delete(courseID, materialID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
		kv("materialId", materialID),
	)
}
