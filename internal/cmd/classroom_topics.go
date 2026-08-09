package cmd

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type ClassroomTopicsCmd struct {
	List   ClassroomTopicsListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List topics"`
	Get    ClassroomTopicsGetCmd    `cmd:"" aliases:"info,show" help:"Get a topic"`
	Create ClassroomTopicsCreateCmd `cmd:"" aliases:"add,new" help:"Create a topic"`
	Update ClassroomTopicsUpdateCmd `cmd:"" aliases:"edit,set" help:"Update a topic"`
	Delete ClassroomTopicsDeleteCmd `cmd:"" aliases:"rm,del,remove" help:"Delete a topic"`
}

type ClassroomTopicsListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

func (c *ClassroomTopicsListCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomPagedList(ctx, flags, classroomPagedListOptions[classroom.Topic]{
		parentName: "courseId", parentID: c.CourseID, max: c.Max, page: c.Page, all: c.All,
		failEmpty: c.FailEmpty, jsonKey: "topics", emptyMessage: "No topics", columns: classroomTopicColumns(),
		fetch: fetchClassroomTopicPage,
	})
}

type ClassroomTopicsGetCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	TopicID  string `arg:"" name:"topicId" help:"Topic ID"`
}

func (c *ClassroomTopicsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	topicID := strings.TrimSpace(c.TopicID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if topicID == "" {
		return usage("empty topicId")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	topic, err := svc.Courses.Topics.Get(courseID, topicID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"topic": topic})
	}

	u.Out().Linef("id\t%s", topic.TopicId)
	u.Out().Linef("name\t%s", topic.Name)
	if topic.UpdateTime != "" {
		u.Out().Linef("updated\t%s", topic.UpdateTime)
	}
	return nil
}

type ClassroomTopicsCreateCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	Name     string `name:"name" help:"Topic name" required:""`
}

func (c *ClassroomTopicsCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}

	topic := &classroom.Topic{Name: name}
	if err := dryRunExit(ctx, flags, "classroom.topics.create", map[string]any{
		"course_id": courseID,
		"topic":     topic,
	}); err != nil {
		return err
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.Topics.Create(courseID, topic).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"topic": created})
	}
	u.Out().Linef("id\t%s", created.TopicId)
	u.Out().Linef("name\t%s", created.Name)
	return nil
}

type ClassroomTopicsUpdateCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	TopicID  string `arg:"" name:"topicId" help:"Topic ID"`
	Name     string `name:"name" help:"Topic name" required:""`
}

func (c *ClassroomTopicsUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	topicID := strings.TrimSpace(c.TopicID)
	name := strings.TrimSpace(c.Name)
	if courseID == "" {
		return usage("empty courseId")
	}
	if topicID == "" {
		return usage("empty topicId")
	}
	if name == "" {
		return usage("empty name")
	}

	topic := &classroom.Topic{Name: name}
	if err := dryRunExit(ctx, flags, "classroom.topics.update", map[string]any{
		"course_id":   courseID,
		"topic_id":    topicID,
		"update_mask": "name",
		"topic":       topic,
	}); err != nil {
		return err
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Topics.Patch(courseID, topicID, topic).UpdateMask("name").Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{"topic": updated})
	}
	u.Out().Linef("id\t%s", updated.TopicId)
	u.Out().Linef("name\t%s", updated.Name)
	return nil
}

type ClassroomTopicsDeleteCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	TopicID  string `arg:"" name:"topicId" help:"Topic ID"`
}

func (c *ClassroomTopicsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	return runClassroomDelete(ctx, flags, c.CourseID, c.TopicID, classroomDeleteOperation{
		op: "classroom.topics.delete", parentName: "courseId", parentPayloadKey: "course_id", parentResultKey: "courseId",
		childName: "topicId", childPayloadKey: "topic_id", childResultKey: "topicId", successResultKey: "deleted",
		action: func(courseID, topicID string) string {
			return fmt.Sprintf("delete topic %s from %s", topicID, courseID)
		},
		delete: func(svc *classroom.Service, courseID, topicID string) error {
			_, err := svc.Courses.Topics.Delete(courseID, topicID).Context(ctx).Do()
			return err
		},
	})
}
