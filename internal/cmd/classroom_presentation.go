package cmd

import (
	"strconv"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/outfmt"
)

type classroomRosterRow struct {
	Role    string
	UserID  string
	Profile *classroom.UserProfile
}

func classroomCourseColumns() []outfmt.Column[*classroom.Course] {
	return []outfmt.Column[*classroom.Course]{
		sanitizedColumn("ID", func(course *classroom.Course) string { return course.Id }),
		sanitizedColumn("NAME", func(course *classroom.Course) string { return course.Name }),
		sanitizedColumn("SECTION", func(course *classroom.Course) string { return course.Section }),
		sanitizedColumn("STATE", func(course *classroom.Course) string { return course.CourseState }),
		sanitizedColumn("OWNER", func(course *classroom.Course) string { return course.OwnerId }),
	}
}

func classroomStudentColumns() []outfmt.Column[*classroom.Student] {
	return []outfmt.Column[*classroom.Student]{
		{Header: "USER_ID", Value: func(student *classroom.Student) string {
			return sanitizeTab(student.UserId)
		}},
		{Header: "EMAIL", Value: func(student *classroom.Student) string {
			return sanitizeTab(profileEmail(student.Profile))
		}},
		{Header: "NAME", Value: func(student *classroom.Student) string {
			return sanitizeTab(profileName(student.Profile))
		}},
	}
}

func classroomTeacherColumns() []outfmt.Column[*classroom.Teacher] {
	return []outfmt.Column[*classroom.Teacher]{
		{Header: "USER_ID", Value: func(teacher *classroom.Teacher) string {
			return sanitizeTab(teacher.UserId)
		}},
		{Header: "EMAIL", Value: func(teacher *classroom.Teacher) string {
			return sanitizeTab(profileEmail(teacher.Profile))
		}},
		{Header: "NAME", Value: func(teacher *classroom.Teacher) string {
			return sanitizeTab(profileName(teacher.Profile))
		}},
	}
}

func classroomRosterColumns() []outfmt.Column[classroomRosterRow] {
	return []outfmt.Column[classroomRosterRow]{
		{Header: "ROLE", Value: func(row classroomRosterRow) string { return row.Role }},
		{Header: "USER_ID", Value: func(row classroomRosterRow) string { return sanitizeTab(row.UserID) }},
		{Header: "EMAIL", Value: func(row classroomRosterRow) string {
			return sanitizeTab(profileEmail(row.Profile))
		}},
		{Header: "NAME", Value: func(row classroomRosterRow) string {
			return sanitizeTab(profileName(row.Profile))
		}},
	}
}

func classroomRosterRows(teachers []*classroom.Teacher, students []*classroom.Student) []classroomRosterRow {
	rows := make([]classroomRosterRow, 0, len(teachers)+len(students))
	for _, teacher := range teachers {
		if teacher != nil {
			rows = append(rows, classroomRosterRow{
				Role:    "teacher",
				UserID:  teacher.UserId,
				Profile: teacher.Profile,
			})
		}
	}
	for _, student := range students {
		if student != nil {
			rows = append(rows, classroomRosterRow{
				Role:    "student",
				UserID:  student.UserId,
				Profile: student.Profile,
			})
		}
	}
	return rows
}

func classroomAnnouncementColumns() []outfmt.Column[*classroom.Announcement] {
	return []outfmt.Column[*classroom.Announcement]{
		{Header: "ID", Value: func(announcement *classroom.Announcement) string {
			return sanitizeTab(announcement.Id)
		}},
		{Header: "STATE", Value: func(announcement *classroom.Announcement) string {
			return sanitizeTab(announcement.State)
		}},
		{Header: "TEXT", Value: func(announcement *classroom.Announcement) string {
			return sanitizeTab(truncateClassroomText(announcement.Text, 50))
		}},
		{Header: "SCHEDULED", Value: func(announcement *classroom.Announcement) string {
			return sanitizeTab(announcement.ScheduledTime)
		}},
		{Header: "UPDATED", Value: func(announcement *classroom.Announcement) string {
			return sanitizeTab(announcement.UpdateTime)
		}},
	}
}

func classroomTopicColumns() []outfmt.Column[*classroom.Topic] {
	return []outfmt.Column[*classroom.Topic]{
		{Header: "TOPIC_ID", Value: func(topic *classroom.Topic) string { return sanitizeTab(topic.TopicId) }},
		{Header: "NAME", Value: func(topic *classroom.Topic) string { return sanitizeTab(topic.Name) }},
		{Header: "UPDATED", Value: func(topic *classroom.Topic) string { return sanitizeTab(topic.UpdateTime) }},
	}
}

func classroomCourseworkColumns() []outfmt.Column[*classroom.CourseWork] {
	return []outfmt.Column[*classroom.CourseWork]{
		{Header: "ID", Value: func(work *classroom.CourseWork) string { return sanitizeTab(work.Id) }},
		{Header: "TITLE", Value: func(work *classroom.CourseWork) string { return sanitizeTab(work.Title) }},
		{Header: "STATE", Value: func(work *classroom.CourseWork) string { return sanitizeTab(work.State) }},
		{Header: "DUE", Value: func(work *classroom.CourseWork) string {
			return sanitizeTab(formatClassroomDue(work.DueDate, work.DueTime))
		}},
		{Header: "TYPE", Value: func(work *classroom.CourseWork) string { return sanitizeTab(work.WorkType) }},
		{Header: "MAX_POINTS", Value: func(work *classroom.CourseWork) string {
			return formatFloatValue(work.MaxPoints)
		}},
	}
}

func classroomMaterialColumns() []outfmt.Column[*classroom.CourseWorkMaterial] {
	return []outfmt.Column[*classroom.CourseWorkMaterial]{
		{Header: "ID", Value: func(material *classroom.CourseWorkMaterial) string {
			return sanitizeTab(material.Id)
		}},
		{Header: "TITLE", Value: func(material *classroom.CourseWorkMaterial) string {
			return sanitizeTab(material.Title)
		}},
		{Header: "STATE", Value: func(material *classroom.CourseWorkMaterial) string {
			return sanitizeTab(material.State)
		}},
		{Header: "UPDATED", Value: func(material *classroom.CourseWorkMaterial) string {
			return sanitizeTab(material.UpdateTime)
		}},
	}
}

func classroomInvitationColumns() []outfmt.Column[*classroom.Invitation] {
	return []outfmt.Column[*classroom.Invitation]{
		{Header: "ID", Value: func(invitation *classroom.Invitation) string {
			return sanitizeTab(invitation.Id)
		}},
		{Header: "COURSE_ID", Value: func(invitation *classroom.Invitation) string {
			return sanitizeTab(invitation.CourseId)
		}},
		{Header: "USER_ID", Value: func(invitation *classroom.Invitation) string {
			return sanitizeTab(invitation.UserId)
		}},
		{Header: "ROLE", Value: func(invitation *classroom.Invitation) string {
			return sanitizeTab(invitation.Role)
		}},
	}
}

func classroomSubmissionColumns() []outfmt.Column[*classroom.StudentSubmission] {
	return []outfmt.Column[*classroom.StudentSubmission]{
		{Header: "ID", Value: func(submission *classroom.StudentSubmission) string {
			return sanitizeTab(submission.Id)
		}},
		{Header: "USER_ID", Value: func(submission *classroom.StudentSubmission) string {
			return sanitizeTab(submission.UserId)
		}},
		{Header: "STATE", Value: func(submission *classroom.StudentSubmission) string {
			return sanitizeTab(submission.State)
		}},
		{Header: "LATE", Value: func(submission *classroom.StudentSubmission) string {
			return strconv.FormatBool(submission.Late)
		}},
		{Header: "DRAFT", Value: func(submission *classroom.StudentSubmission) string {
			return formatFloatValue(submission.DraftGrade)
		}},
		{Header: "ASSIGNED", Value: func(submission *classroom.StudentSubmission) string {
			return formatFloatValue(submission.AssignedGrade)
		}},
		{Header: "UPDATED", Value: func(submission *classroom.StudentSubmission) string {
			return sanitizeTab(submission.UpdateTime)
		}},
	}
}

func classroomGuardianColumns() []outfmt.Column[*classroom.Guardian] {
	return []outfmt.Column[*classroom.Guardian]{
		{Header: "GUARDIAN_ID", Value: func(guardian *classroom.Guardian) string {
			return sanitizeTab(guardian.GuardianId)
		}},
		{Header: "EMAIL", Value: func(guardian *classroom.Guardian) string {
			return sanitizeTab(profileEmail(guardian.GuardianProfile))
		}},
		{Header: "NAME", Value: func(guardian *classroom.Guardian) string {
			return sanitizeTab(profileName(guardian.GuardianProfile))
		}},
	}
}

func classroomGuardianInvitationColumns() []outfmt.Column[*classroom.GuardianInvitation] {
	return []outfmt.Column[*classroom.GuardianInvitation]{
		{Header: "INVITATION_ID", Value: func(invitation *classroom.GuardianInvitation) string {
			return sanitizeTab(invitation.InvitationId)
		}},
		{Header: "EMAIL", Value: func(invitation *classroom.GuardianInvitation) string {
			return sanitizeTab(invitation.InvitedEmailAddress)
		}},
		{Header: "STATE", Value: func(invitation *classroom.GuardianInvitation) string {
			return sanitizeTab(invitation.State)
		}},
		{Header: "CREATED", Value: func(invitation *classroom.GuardianInvitation) string {
			return sanitizeTab(invitation.CreationTime)
		}},
	}
}
