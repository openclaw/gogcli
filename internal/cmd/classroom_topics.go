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

	fetch := func(pageToken string) ([]*classroom.Topic, string, error) {
		call := svc.Courses.Topics.List(courseID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Topic, resp.NextPageToken, nil
	}

	var topics []*classroom.Topic
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		topics = all
	} else {
		var err error
		topics, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"topics":        topics,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(topics) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(topics) == 0 {
		u.Err().Println("No topics")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TOPIC_ID\tNAME\tUPDATED")
	for _, topic := range topics {
		if topic == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(topic.TopicId),
			sanitizeTab(topic.Name),
			sanitizeTab(topic.UpdateTime),
		)
	}
	printNextPageHint(u, nextPageToken)
	return nil
}

type ClassroomTopicsGetCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	TopicID  string `arg:"" name:"topicId" help:"Topic ID"`
}

func (c *ClassroomTopicsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	topicID := strings.TrimSpace(c.TopicID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if topicID == "" {
		return usage("empty topicId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	topic, err := svc.Courses.Topics.Get(courseID, topicID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"topic": topic})
	}

	u.Out().Printf("id\t%s", topic.TopicId)
	u.Out().Printf("name\t%s", topic.Name)
	if topic.UpdateTime != "" {
		u.Out().Printf("updated\t%s", topic.UpdateTime)
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	created, err := svc.Courses.Topics.Create(courseID, topic).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"topic": created})
	}
	u.Out().Printf("id\t%s", created.TopicId)
	u.Out().Printf("name\t%s", created.Name)
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

	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	updated, err := svc.Courses.Topics.Patch(courseID, topicID, topic).UpdateMask("name").Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"topic": updated})
	}
	u.Out().Printf("id\t%s", updated.TopicId)
	u.Out().Printf("name\t%s", updated.Name)
	return nil
}

type ClassroomTopicsDeleteCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	TopicID  string `arg:"" name:"topicId" help:"Topic ID"`
}

func (c *ClassroomTopicsDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	courseID := strings.TrimSpace(c.CourseID)
	topicID := strings.TrimSpace(c.TopicID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if topicID == "" {
		return usage("empty topicId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("delete topic %s from %s", topicID, courseID)); err != nil {
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

	if _, err := svc.Courses.Topics.Delete(courseID, topicID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("deleted", true),
		kv("courseId", courseID),
		kv("topicId", topicID),
	)
}
