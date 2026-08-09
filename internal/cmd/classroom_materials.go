package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
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
	return runClassroomTopicList(ctx, flags, classroomTopicListOptions[classroom.CourseWorkMaterial]{
		courseID: c.CourseID, states: c.States, topic: c.Topic, orderBy: c.OrderBy,
		max: c.Max, page: c.Page, all: c.All, failEmpty: c.FailEmpty, scanPages: c.ScanPages,
		jsonKey: "materials", emptyMessage: "No materials", columns: classroomMaterialColumns(),
		fetch: fetchClassroomMaterialPage, topicID: classroomMaterialTopicID,
	})
}

type ClassroomMaterialsGetCmd struct {
	CourseID   string `arg:"" name:"courseId" help:"Course ID or alias"`
	MaterialID string `arg:"" name:"materialId" help:"Material ID"`
}

func (c *ClassroomMaterialsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	materialID := strings.TrimSpace(c.MaterialID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if materialID == "" {
		return usage("empty materialId")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	material, err := svc.Courses.CourseWorkMaterials.Get(courseID, materialID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"material": material})
	}

	u.Out().Linef("id\t%s", material.Id)
	u.Out().Linef("title\t%s", material.Title)
	if material.Description != "" {
		u.Out().Linef("description\t%s", material.Description)
	}
	u.Out().Linef("state\t%s", material.State)
	if material.TopicId != "" {
		u.Out().Linef("topic_id\t%s", material.TopicId)
	}
	if material.ScheduledTime != "" {
		u.Out().Linef("scheduled\t%s", material.ScheduledTime)
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
	plan, err := buildClassroomMaterialCreatePlan(classroomMaterialInput{
		CourseID:    c.CourseID,
		Title:       c.Title,
		Description: c.Description,
		State:       c.State,
		Scheduled:   c.Scheduled,
		TopicID:     c.TopicID,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.materials.create", map[string]any{
		"course_id": plan.CourseID,
		"material":  plan.Material,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.CourseWorkMaterials.Create(plan.CourseID, plan.Material).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"material": created})
	}
	u.Out().Linef("id\t%s", created.Id)
	u.Out().Linef("title\t%s", created.Title)
	u.Out().Linef("state\t%s", created.State)
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
	plan, err := buildClassroomMaterialUpdatePlan(classroomMaterialUpdateInput{
		classroomMaterialInput: classroomMaterialInput{
			CourseID:    c.CourseID,
			Title:       c.Title,
			Description: c.Description,
			State:       c.State,
			Scheduled:   c.Scheduled,
			TopicID:     c.TopicID,
		},
		MaterialID: c.MaterialID,
	})
	if err != nil {
		return err
	}

	if dryRunErr := dryRunExit(ctx, flags, "classroom.materials.update", map[string]any{
		"course_id":     plan.CourseID,
		"material_id":   plan.MaterialID,
		"update_mask":   plan.UpdateMask,
		"update_fields": plan.UpdateFields,
		"material":      plan.Material,
	}); dryRunErr != nil {
		return dryRunErr
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.CourseWorkMaterials.Patch(plan.CourseID, plan.MaterialID, plan.Material).UpdateMask(plan.UpdateMask).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"material": updated})
	}
	u.Out().Linef("id\t%s", updated.Id)
	u.Out().Linef("title\t%s", updated.Title)
	u.Out().Linef("state\t%s", updated.State)
	return nil
}

type ClassroomMaterialsDeleteCmd struct {
	CourseID   string `arg:"" name:"courseId" help:"Course ID or alias"`
	MaterialID string `arg:"" name:"materialId" help:"Material ID"`
}

func (c *ClassroomMaterialsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomDelete(ctx, flags, c.CourseID, c.MaterialID, classroomDeleteOperation{
		op: "classroom.materials.delete", parentName: "courseId", parentPayloadKey: "course_id", parentResultKey: "courseId",
		childName: "materialId", childPayloadKey: "material_id", childResultKey: "materialId", successResultKey: "deleted",
		action: func(courseID, materialID string) string {
			return fmt.Sprintf("delete material %s from %s", materialID, courseID)
		},
		delete: func(svc *classroom.Service, courseID, materialID string) error {
			_, err := svc.Courses.CourseWorkMaterials.Delete(courseID, materialID).Context(ctx).Do()
			return err
		},
	})
}
