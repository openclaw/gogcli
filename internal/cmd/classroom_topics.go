package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/degree-analytics/ratatosk/internal/outfmt"
	"github.com/degree-analytics/ratatosk/internal/ui"
)

type ClassroomTopicsCmd struct {
	List   ClassroomTopicsListCmd   `cmd:"" default:"withargs" help:"List topics"`
	Get    ClassroomTopicsGetCmd    `cmd:"" help:"Get a topic"`
	Create ClassroomTopicsCreateCmd `cmd:"" help:"Create a topic"`
	Update ClassroomTopicsUpdateCmd `cmd:"" help:"Update a topic"`
	Delete ClassroomTopicsDeleteCmd `cmd:"" help:"Delete a topic" aliases:"rm"`
}

type ClassroomTopicsListCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page     string `name:"page" help:"Page token"`
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

	resp, err := svc.Courses.Topics.List(courseID).PageSize(c.Max).PageToken(c.Page).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"topics":        resp.Topic,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Topic) == 0 {
		u.Err().Println("No topics")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "TOPIC_ID\tNAME\tUPDATED")
	for _, topic := range resp.Topic {
		if topic == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(topic.TopicId),
			sanitizeTab(topic.Name),
			sanitizeTab(topic.UpdateTime),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
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
		return outfmt.WriteJSON(os.Stdout, map[string]any{"topic": topic})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return usage("empty name")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	topic := &classroom.Topic{Name: name}
	created, err := svc.Courses.Topics.Create(courseID, topic).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"topic": created})
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
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
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

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	topic := &classroom.Topic{Name: name}
	updated, err := svc.Courses.Topics.Patch(courseID, topicID, topic).UpdateMask("name").Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"topic": updated})
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

	err = confirmDestructive(ctx, flags, fmt.Sprintf("delete topic %s from %s", topicID, courseID))
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

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted":  true,
			"courseId": courseID,
			"topicId":  topicID,
		})
	}
	u.Out().Printf("deleted\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	u.Out().Printf("topic_id\t%s", topicID)
	return nil
}
