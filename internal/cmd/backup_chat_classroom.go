package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/api/chat/v1"
	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/backup"
)

type chatBackupMessage struct {
	SpaceName string        `json:"spaceName"`
	Message   *chat.Message `json:"message"`
}

type classroomBackupTopic struct {
	CourseID string           `json:"courseId"`
	Topic    *classroom.Topic `json:"topic"`
}

type classroomBackupAnnouncement struct {
	CourseID     string                  `json:"courseId"`
	Announcement *classroom.Announcement `json:"announcement"`
}

type classroomBackupCourseWork struct {
	CourseID   string                `json:"courseId"`
	CourseWork *classroom.CourseWork `json:"courseWork"`
}

type classroomBackupMaterial struct {
	CourseID string                        `json:"courseId"`
	Material *classroom.CourseWorkMaterial `json:"material"`
}

type classroomBackupSubmission struct {
	CourseID     string                       `json:"courseId"`
	CourseWorkID string                       `json:"courseWorkId"`
	Submission   *classroom.StudentSubmission `json:"submission"`
}

func buildChatBackupSnapshot(ctx context.Context, flags *RootFlags, shardMaxRows int) (backup.Snapshot, error) {
	account, err := requireAccount(flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	if workspaceErr := requireWorkspaceAccount(account); workspaceErr != nil {
		return backup.Snapshot{}, workspaceErr
	}
	svc, err := chatService(ctx, account)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	spaces, err := fetchBackupChatSpaces(ctx, svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	messages, err := fetchBackupChatMessages(ctx, svc, spaces)
	if err != nil {
		return backup.Snapshot{}, err
	}
	spaceShard, err := backup.NewJSONLShard(backupServiceChat, "spaces", accountHash, fmt.Sprintf("data/chat/%s/spaces.jsonl.gz.age", accountHash), spaces)
	if err != nil {
		return backup.Snapshot{}, err
	}
	messageShards, err := buildBackupShards(backupServiceChat, "messages", accountHash, fmt.Sprintf("data/chat/%s/messages", accountHash), messages, shardMaxRows)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards := append([]backup.PlainShard{spaceShard}, messageShards...)
	return backup.Snapshot{
		Services: []string{backupServiceChat},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"chat.spaces":   len(spaces),
			"chat.messages": len(messages),
		},
		Shards: shards,
	}, nil
}

func buildClassroomBackupSnapshot(ctx context.Context, flags *RootFlags, shardMaxRows int) (backup.Snapshot, error) {
	account, svc, err := requireClassroomService(ctx, flags)
	if err != nil {
		return backup.Snapshot{}, err
	}
	accountHash := backupAccountHash(account)
	courses, err := fetchBackupClassroomCourses(ctx, svc)
	if err != nil {
		return backup.Snapshot{}, err
	}
	topics, announcements, coursework, materials, submissions, err := fetchBackupClassroomChildren(ctx, svc, courses)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards := make([]backup.PlainShard, 0, 6)
	courseShard, err := backup.NewJSONLShard(backupServiceClassroom, "courses", accountHash, fmt.Sprintf("data/classroom/%s/courses.jsonl.gz.age", accountHash), courses)
	if err != nil {
		return backup.Snapshot{}, err
	}
	shards = append(shards, courseShard)
	for _, part := range []struct {
		kind string
		rows any
	}{
		{"topics", topics},
		{"announcements", announcements},
		{"coursework", coursework},
		{"materials", materials},
		{"submissions", submissions},
	} {
		shard, shardErr := buildBackupShardsAny(backupServiceClassroom, part.kind, accountHash, fmt.Sprintf("data/classroom/%s/%s", accountHash, part.kind), part.rows, shardMaxRows)
		if shardErr != nil {
			return backup.Snapshot{}, shardErr
		}
		shards = append(shards, shard...)
	}
	return backup.Snapshot{
		Services: []string{backupServiceClassroom},
		Accounts: []string{accountHash},
		Counts: map[string]int{
			"classroom.courses":       len(courses),
			"classroom.topics":        len(topics),
			"classroom.announcements": len(announcements),
			"classroom.coursework":    len(coursework),
			"classroom.materials":     len(materials),
			"classroom.submissions":   len(submissions),
		},
		Shards: shards,
	}, nil
}

func fetchBackupChatSpaces(ctx context.Context, svc *chat.Service) ([]*chat.Space, error) {
	fetch := func(pageToken string) ([]*chat.Space, string, error) {
		call := svc.Spaces.List().PageSize(1000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Spaces, resp.NextPageToken, nil
	}
	out, err := collectAllPages("", fetch)
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func fetchBackupChatMessages(ctx context.Context, svc *chat.Service, spaces []*chat.Space) ([]chatBackupMessage, error) {
	var out []chatBackupMessage
	for _, space := range spaces {
		if space == nil || strings.TrimSpace(space.Name) == "" {
			continue
		}
		fetch := func(pageToken string) ([]*chat.Message, string, error) {
			call := svc.Spaces.Messages.List(space.Name).PageSize(1000).Context(ctx)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}
			resp, err := call.Do()
			if err != nil {
				return nil, "", fmt.Errorf("chat messages %s: %w", space.Name, err)
			}
			return resp.Messages, resp.NextPageToken, nil
		}
		messages, err := collectAllPages("", fetch)
		if err != nil {
			return nil, err
		}
		for _, message := range messages {
			out = append(out, chatBackupMessage{SpaceName: space.Name, Message: message})
		}
	}
	return out, nil
}

func fetchBackupClassroomCourses(ctx context.Context, svc *classroom.Service) ([]*classroom.Course, error) {
	fetch := func(pageToken string) ([]*classroom.Course, string, error) {
		call := svc.Courses.List().PageSize(100).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", err
		}
		return resp.Courses, resp.NextPageToken, nil
	}
	out, err := collectAllPages("", fetch)
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out, nil
}

func fetchBackupClassroomChildren(ctx context.Context, svc *classroom.Service, courses []*classroom.Course) ([]classroomBackupTopic, []classroomBackupAnnouncement, []classroomBackupCourseWork, []classroomBackupMaterial, []classroomBackupSubmission, error) {
	var topics []classroomBackupTopic
	var announcements []classroomBackupAnnouncement
	var coursework []classroomBackupCourseWork
	var materials []classroomBackupMaterial
	var submissions []classroomBackupSubmission
	for _, course := range courses {
		if course == nil || strings.TrimSpace(course.Id) == "" {
			continue
		}
		courseID := course.Id
		courseTopics, err := fetchClassroomTopicsBestEffort(ctx, svc, courseID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, topic := range courseTopics {
			topics = append(topics, classroomBackupTopic{CourseID: courseID, Topic: topic})
		}
		courseAnnouncements, err := fetchClassroomAnnouncementsBestEffort(ctx, svc, courseID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, announcement := range courseAnnouncements {
			announcements = append(announcements, classroomBackupAnnouncement{CourseID: courseID, Announcement: announcement})
		}
		courseWork, err := fetchClassroomCourseWorkBestEffort(ctx, svc, courseID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, work := range courseWork {
			coursework = append(coursework, classroomBackupCourseWork{CourseID: courseID, CourseWork: work})
		}
		courseMaterials, err := fetchClassroomMaterialsBestEffort(ctx, svc, courseID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, material := range courseMaterials {
			materials = append(materials, classroomBackupMaterial{CourseID: courseID, Material: material})
		}
		courseSubmissions, err := fetchClassroomSubmissionsBestEffort(ctx, svc, courseID)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		for _, submission := range courseSubmissions {
			courseWorkID := ""
			if submission != nil {
				courseWorkID = submission.CourseWorkId
			}
			submissions = append(submissions, classroomBackupSubmission{CourseID: courseID, CourseWorkID: courseWorkID, Submission: submission})
		}
	}
	return topics, announcements, coursework, materials, submissions, nil
}

func fetchClassroomTopicsBestEffort(ctx context.Context, svc *classroom.Service, courseID string) ([]*classroom.Topic, error) {
	return collectAllPages("", func(pageToken string) ([]*classroom.Topic, string, error) {
		resp, err := svc.Courses.Topics.List(courseID).PageSize(100).PageToken(pageToken).Context(ctx).Do()
		if err != nil {
			return nil, "", nil //nolint:nilerr // End best-effort listing without losing earlier pages.
		}
		return resp.Topic, resp.NextPageToken, nil
	})
}

func fetchClassroomAnnouncementsBestEffort(ctx context.Context, svc *classroom.Service, courseID string) ([]*classroom.Announcement, error) {
	return collectAllPages("", func(pageToken string) ([]*classroom.Announcement, string, error) {
		resp, err := svc.Courses.Announcements.List(courseID).PageSize(100).PageToken(pageToken).Context(ctx).Do()
		if err != nil {
			return nil, "", nil //nolint:nilerr // End best-effort listing without losing earlier pages.
		}
		return resp.Announcements, resp.NextPageToken, nil
	})
}

func fetchClassroomCourseWorkBestEffort(ctx context.Context, svc *classroom.Service, courseID string) ([]*classroom.CourseWork, error) {
	return collectAllPages("", func(pageToken string) ([]*classroom.CourseWork, string, error) {
		resp, err := svc.Courses.CourseWork.List(courseID).PageSize(100).PageToken(pageToken).Context(ctx).Do()
		if err != nil {
			return nil, "", nil //nolint:nilerr // End best-effort listing without losing earlier pages.
		}
		return resp.CourseWork, resp.NextPageToken, nil
	})
}

func fetchClassroomMaterialsBestEffort(ctx context.Context, svc *classroom.Service, courseID string) ([]*classroom.CourseWorkMaterial, error) {
	return collectAllPages("", func(pageToken string) ([]*classroom.CourseWorkMaterial, string, error) {
		resp, err := svc.Courses.CourseWorkMaterials.List(courseID).PageSize(100).PageToken(pageToken).Context(ctx).Do()
		if err != nil {
			return nil, "", nil //nolint:nilerr // End best-effort listing without losing earlier pages.
		}
		return resp.CourseWorkMaterial, resp.NextPageToken, nil
	})
}

func fetchClassroomSubmissionsBestEffort(ctx context.Context, svc *classroom.Service, courseID string) ([]*classroom.StudentSubmission, error) {
	return collectAllPages("", func(pageToken string) ([]*classroom.StudentSubmission, string, error) {
		resp, err := svc.Courses.CourseWork.StudentSubmissions.List(courseID, "-").PageSize(100).PageToken(pageToken).Context(ctx).Do()
		if err != nil {
			return nil, "", nil //nolint:nilerr // End best-effort listing without losing earlier pages.
		}
		return resp.StudentSubmissions, resp.NextPageToken, nil
	})
}
