package cmd

import (
	"context"
	"strings"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
	"github.com/openclaw/gogcli/internal/ui"
)

type classroomPagedListOptions[T any] struct {
	parentName   string
	parentID     string
	max          int64
	page         string
	all          bool
	failEmpty    bool
	jsonKey      string
	emptyMessage string
	hintOnEmpty  bool
	columns      []outfmt.Column[*T]
	fetch        func(context.Context, *classroom.Service, string, int64, string) ([]*T, string, error)
}

func runClassroomPagedList[T any](ctx context.Context, flags *RootFlags, options classroomPagedListOptions[T]) error {
	parentID := strings.TrimSpace(options.parentID)
	if options.parentName != "" && parentID == "" {
		return usage("empty " + options.parentName)
	}
	if options.max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}
	items, nextPageToken, err := loadPagedItems(options.page, options.all, func(pageToken string) ([]*T, string, error) {
		return options.fetch(ctx, svc, parentID, options.max, pageToken)
	})
	if err != nil {
		return wrapClassroomError(err)
	}

	return writeClassroomPagedList(
		ctx,
		options.jsonKey,
		items,
		nextPageToken,
		options.emptyMessage,
		options.failEmpty,
		options.hintOnEmpty,
		options.columns,
	)
}

type classroomTopicListOptions[T any] struct {
	courseID     string
	states       string
	topic        string
	orderBy      string
	max          int64
	page         string
	all          bool
	failEmpty    bool
	scanPages    int
	jsonKey      string
	emptyMessage string
	columns      []outfmt.Column[*T]
	fetch        func(context.Context, *classroom.Service, string, int64, string, []string, string) ([]*T, string, error)
	topicID      func(*T) string
}

func runClassroomTopicList[T any](ctx context.Context, flags *RootFlags, options classroomTopicListOptions[T]) error {
	courseID := strings.TrimSpace(options.courseID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if options.max <= 0 {
		return usage("max must be > 0")
	}

	_, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return wrapClassroomError(err)
	}
	states := upperClassroomStates(options.states)
	orderBy := strings.TrimSpace(options.orderBy)
	fetch := func(pageToken string) ([]*T, string, error) {
		return options.fetch(ctx, svc, courseID, options.max, pageToken, states, orderBy)
	}

	var items []*T
	var nextPageToken string
	if options.all {
		items, _, err = loadPagedItems(options.page, true, fetch)
		if err == nil {
			items = filterClassroomTopicItems(items, options.topic, options.topicID)
		}
	} else {
		items, nextPageToken, err = scanClassroomTopicPages(options.topic, options.page, options.scanPages, fetch, options.topicID)
	}
	if err != nil {
		return wrapClassroomError(err)
	}

	return writeClassroomPagedList(ctx, options.jsonKey, items, nextPageToken, options.emptyMessage, options.failEmpty, true, options.columns)
}

func filterClassroomTopicItems[T any](items []*T, rawTopic string, topicID func(*T) string) []*T {
	topic := strings.TrimSpace(rawTopic)
	if topic == "" {
		return items
	}
	filtered := items[:0]
	for _, item := range items {
		if item != nil && topicID(item) == topic {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func upperClassroomStates(raw string) []string {
	states := splitCSV(raw)
	for i := range states {
		states[i] = strings.ToUpper(states[i])
	}
	return states
}

func fetchClassroomCourseworkPage(ctx context.Context, svc *classroom.Service, courseID string, pageSize int64, page string, states []string, orderBy string) ([]*classroom.CourseWork, string, error) {
	call := svc.Courses.CourseWork.List(courseID).PageSize(pageSize).PageToken(page).Context(ctx)
	if len(states) > 0 {
		call.CourseWorkStates(states...)
	}
	if orderBy != "" {
		call.OrderBy(orderBy)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.CourseWork, resp.NextPageToken, nil
}

func fetchClassroomMaterialPage(ctx context.Context, svc *classroom.Service, courseID string, pageSize int64, page string, states []string, orderBy string) ([]*classroom.CourseWorkMaterial, string, error) {
	call := svc.Courses.CourseWorkMaterials.List(courseID).PageSize(pageSize).PageToken(page).Context(ctx)
	if len(states) > 0 {
		call.CourseWorkMaterialStates(states...)
	}
	if orderBy != "" {
		call.OrderBy(orderBy)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.CourseWorkMaterial, resp.NextPageToken, nil
}

func classroomCourseworkTopicID(work *classroom.CourseWork) string {
	if work == nil {
		return ""
	}
	return work.TopicId
}

func classroomMaterialTopicID(material *classroom.CourseWorkMaterial) string {
	if material == nil {
		return ""
	}
	return material.TopicId
}

func fetchClassroomStudentPage(ctx context.Context, svc *classroom.Service, courseID string, pageSize int64, pageToken string) ([]*classroom.Student, string, error) {
	call := svc.Courses.Students.List(courseID).PageSize(pageSize).Context(ctx)
	if strings.TrimSpace(pageToken) != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.Students, resp.NextPageToken, nil
}

func fetchClassroomTeacherPage(ctx context.Context, svc *classroom.Service, courseID string, pageSize int64, pageToken string) ([]*classroom.Teacher, string, error) {
	call := svc.Courses.Teachers.List(courseID).PageSize(pageSize).Context(ctx)
	if strings.TrimSpace(pageToken) != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.Teachers, resp.NextPageToken, nil
}

func fetchClassroomTopicPage(ctx context.Context, svc *classroom.Service, courseID string, pageSize int64, pageToken string) ([]*classroom.Topic, string, error) {
	call := svc.Courses.Topics.List(courseID).PageSize(pageSize).Context(ctx)
	if strings.TrimSpace(pageToken) != "" {
		call = call.PageToken(pageToken)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, "", err
	}
	return resp.Topic, resp.NextPageToken, nil
}

func nonNilClassroomItems[T any](items []*T) []*T {
	if items == nil {
		return []*T{}
	}
	return items
}

func compactClassroomRows[T any](items []*T) []*T {
	rows := make([]*T, 0, len(items))
	for _, item := range items {
		if item != nil {
			rows = append(rows, item)
		}
	}
	return rows
}

func writeClassroomPagedList[T any](
	ctx context.Context,
	jsonKey string,
	items []*T,
	nextPageToken string,
	emptyMessage string,
	failEmpty bool,
	hintOnEmpty bool,
	columns []outfmt.Column[*T],
) error {
	items = nonNilClassroomItems(items)
	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, stdoutWriter(ctx), map[string]any{
			jsonKey:         items,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(items) == 0 {
			return failEmptyExit(failEmpty)
		}
		return nil
	}

	u := ui.FromContext(ctx)
	if len(items) == 0 {
		u.Err().Println(emptyMessage)
		if hintOnEmpty {
			printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
		}
		return failEmptyExit(failEmpty)
	}

	if err := outfmt.WriteTable(ctx, stdoutWriter(ctx), compactClassroomRows(items), columns); err != nil {
		return err
	}
	printNextPageHintWithAll(u, nextPageToken, "--all/--all-pages")
	return nil
}
