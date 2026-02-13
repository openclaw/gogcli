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

type ClassroomStudentsCmd struct {
	List   ClassroomStudentsListCmd   `cmd:"" default:"withargs" help:"List students"`
	Get    ClassroomStudentsGetCmd    `cmd:"" help:"Get a student"`
	Add    ClassroomStudentsAddCmd    `cmd:"" help:"Add a student"`
	Remove ClassroomStudentsRemoveCmd `cmd:"" help:"Remove a student" aliases:"delete,rm"`
}

type ClassroomStudentsListCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page     string `name:"page" help:"Page token"`
}

func (c *ClassroomStudentsListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.Courses.Students.List(courseID).PageSize(c.Max).PageToken(c.Page).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"students":      resp.Students,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Students) == 0 {
		u.Err().Println("No students")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "USER_ID\tEMAIL\tNAME")
	for _, student := range resp.Students {
		if student == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(student.UserId),
			sanitizeTab(profileEmail(student.Profile)),
			sanitizeTab(profileName(student.Profile)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ClassroomStudentsGetCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID   string `arg:"" name:"userId" help:"Student user ID or email"`
}

func (c *ClassroomStudentsGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	student, err := svc.Courses.Students.Get(courseID, userID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"student": student})
	}

	u.Out().Printf("user_id\t%s", student.UserId)
	u.Out().Printf("email\t%s", profileEmail(student.Profile))
	u.Out().Printf("name\t%s", profileName(student.Profile))
	if student.StudentWorkFolder != nil {
		u.Out().Printf("work_folder\t%s", student.StudentWorkFolder.Id)
	}
	return nil
}

type ClassroomStudentsAddCmd struct {
	CourseID       string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID         string `arg:"" name:"userId" help:"Student user ID or email"`
	EnrollmentCode string `name:"enrollment-code" help:"Enrollment code"`
}

func (c *ClassroomStudentsAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	student := &classroom.Student{UserId: userID}
	call := svc.Courses.Students.Create(courseID, student).Context(ctx)
	if code := strings.TrimSpace(c.EnrollmentCode); code != "" {
		call.EnrollmentCode(code)
	}
	created, err := call.Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"student": created})
	}
	u.Out().Printf("user_id\t%s", created.UserId)
	u.Out().Printf("email\t%s", profileEmail(created.Profile))
	u.Out().Printf("name\t%s", profileName(created.Profile))
	return nil
}

type ClassroomStudentsRemoveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID   string `arg:"" name:"userId" help:"Student user ID or email"`
}

func (c *ClassroomStudentsRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	err = confirmDestructive(ctx, flags, fmt.Sprintf("remove student %s from %s", userID, courseID))
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Courses.Students.Delete(courseID, userID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"removed":  true,
			"courseId": courseID,
			"userId":   userID,
		})
	}
	u.Out().Printf("removed\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	u.Out().Printf("user_id\t%s", userID)
	return nil
}

type ClassroomTeachersCmd struct {
	List   ClassroomTeachersListCmd   `cmd:"" default:"withargs" help:"List teachers"`
	Get    ClassroomTeachersGetCmd    `cmd:"" help:"Get a teacher"`
	Add    ClassroomTeachersAddCmd    `cmd:"" help:"Add a teacher"`
	Remove ClassroomTeachersRemoveCmd `cmd:"" help:"Remove a teacher" aliases:"delete,rm"`
}

type ClassroomTeachersListCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page     string `name:"page" help:"Page token"`
}

func (c *ClassroomTeachersListCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	resp, err := svc.Courses.Teachers.List(courseID).PageSize(c.Max).PageToken(c.Page).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"teachers":      resp.Teachers,
			"nextPageToken": resp.NextPageToken,
		})
	}

	if len(resp.Teachers) == 0 {
		u.Err().Println("No teachers")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "USER_ID\tEMAIL\tNAME")
	for _, teacher := range resp.Teachers {
		if teacher == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(teacher.UserId),
			sanitizeTab(profileEmail(teacher.Profile)),
			sanitizeTab(profileName(teacher.Profile)),
		)
	}
	printNextPageHint(u, resp.NextPageToken)
	return nil
}

type ClassroomTeachersGetCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID   string `arg:"" name:"userId" help:"Teacher user ID or email"`
}

func (c *ClassroomTeachersGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	teacher, err := svc.Courses.Teachers.Get(courseID, userID).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"teacher": teacher})
	}

	u.Out().Printf("user_id\t%s", teacher.UserId)
	u.Out().Printf("email\t%s", profileEmail(teacher.Profile))
	u.Out().Printf("name\t%s", profileName(teacher.Profile))
	return nil
}

type ClassroomTeachersAddCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID   string `arg:"" name:"userId" help:"Teacher user ID or email"`
}

func (c *ClassroomTeachersAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	teacher := &classroom.Teacher{UserId: userID}
	created, err := svc.Courses.Teachers.Create(courseID, teacher).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"teacher": created})
	}
	u.Out().Printf("user_id\t%s", created.UserId)
	u.Out().Printf("email\t%s", profileEmail(created.Profile))
	u.Out().Printf("name\t%s", profileName(created.Profile))
	return nil
}

type ClassroomTeachersRemoveCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	UserID   string `arg:"" name:"userId" help:"Teacher user ID or email"`
}

func (c *ClassroomTeachersRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	err = confirmDestructive(ctx, flags, fmt.Sprintf("remove teacher %s from %s", userID, courseID))
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	if _, err := svc.Courses.Teachers.Delete(courseID, userID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"removed":  true,
			"courseId": courseID,
			"userId":   userID,
		})
	}
	u.Out().Printf("removed\ttrue")
	u.Out().Printf("course_id\t%s", courseID)
	u.Out().Printf("user_id\t%s", userID)
	return nil
}

type ClassroomRosterCmd struct {
	CourseID string `arg:"" name:"courseId" help:"Course ID or alias"`
	Students bool   `name:"students" help:"Include students"`
	Teachers bool   `name:"teachers" help:"Include teachers"`
	Max      int64  `name:"max" aliases:"limit" help:"Max results (per role)" default:"100"`
	Page     string `name:"page" help:"Page token (per role)"`
}

func (c *ClassroomRosterCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}
	courseID := strings.TrimSpace(c.CourseID)
	if courseID == "" {
		return usage("empty courseId")
	}

	includeStudents := c.Students || (!c.Students && !c.Teachers)
	includeTeachers := c.Teachers || (!c.Students && !c.Teachers)

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return wrapClassroomError(err)
	}

	var studentsResp *classroom.ListStudentsResponse
	var teachersResp *classroom.ListTeachersResponse

	if includeStudents {
		studentsResp, err = svc.Courses.Students.List(courseID).PageSize(c.Max).PageToken(c.Page).Context(ctx).Do()
		if err != nil {
			return wrapClassroomError(err)
		}
	}
	if includeTeachers {
		teachersResp, err = svc.Courses.Teachers.List(courseID).PageSize(c.Max).PageToken(c.Page).Context(ctx).Do()
		if err != nil {
			return wrapClassroomError(err)
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"courseId": courseID}
		if includeStudents {
			payload["students"] = studentsResp.Students
			payload["studentsNextPageToken"] = studentsResp.NextPageToken
		}
		if includeTeachers {
			payload["teachers"] = teachersResp.Teachers
			payload["teachersNextPageToken"] = teachersResp.NextPageToken
		}
		return outfmt.WriteJSON(os.Stdout, payload)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ROLE\tUSER_ID\tEMAIL\tNAME")
	if includeTeachers {
		for _, teacher := range teachersResp.Teachers {
			if teacher == nil {
				continue
			}
			fmt.Fprintf(w, "teacher\t%s\t%s\t%s\n",
				sanitizeTab(teacher.UserId),
				sanitizeTab(profileEmail(teacher.Profile)),
				sanitizeTab(profileName(teacher.Profile)),
			)
		}
		if teachersResp.NextPageToken != "" {
			u.Err().Printf("# Next teachers page: --page %s", teachersResp.NextPageToken)
		}
	}
	if includeStudents {
		for _, student := range studentsResp.Students {
			if student == nil {
				continue
			}
			fmt.Fprintf(w, "student\t%s\t%s\t%s\n",
				sanitizeTab(student.UserId),
				sanitizeTab(profileEmail(student.Profile)),
				sanitizeTab(profileName(student.Profile)),
			)
		}
		if studentsResp.NextPageToken != "" {
			u.Err().Printf("# Next students page: --page %s", studentsResp.NextPageToken)
		}
	}
	return nil
}
