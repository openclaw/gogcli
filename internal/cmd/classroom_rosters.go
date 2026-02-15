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

type ClassroomStudentsCmd struct {
	List   ClassroomStudentsListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List students"`
	Get    ClassroomStudentsGetCmd    `cmd:"" aliases:"info,show" help:"Get a student"`
	Add    ClassroomStudentsAddCmd    `cmd:"" aliases:"create,new" help:"Add a student"`
	Remove ClassroomStudentsRemoveCmd `cmd:"" aliases:"delete,rm,del,remove" help:"Remove a student"`
}

type ClassroomStudentsListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*classroom.Student, string, error) {
		call := svc.Courses.Students.List(courseID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Students, resp.NextPageToken, nil
	}

	var students []*classroom.Student
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		students = all
	} else {
		var err error
		students, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"students":      students,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(students) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(students) == 0 {
		u.Err().Println("No students")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "USER_ID\tEMAIL\tNAME")
	for _, student := range students {
		if student == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(student.UserId),
			sanitizeTab(profileEmail(student.Profile)),
			sanitizeTab(profileName(student.Profile)),
		)
	}
	printNextPageHint(u, nextPageToken)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"student": student})
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
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	if err := dryRunExit(ctx, flags, "classroom.students.add", map[string]any{
		"course_id":       courseID,
		"user_id":         userID,
		"enrollment_code": strings.TrimSpace(c.EnrollmentCode),
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"student": created})
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
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove student %s from %s", userID, courseID)); err != nil {
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

	if _, err := svc.Courses.Students.Delete(courseID, userID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("removed", true),
		kv("courseId", courseID),
		kv("userId", userID),
	)
}

type ClassroomTeachersCmd struct {
	List   ClassroomTeachersListCmd   `cmd:"" default:"withargs" aliases:"ls" help:"List teachers"`
	Get    ClassroomTeachersGetCmd    `cmd:"" aliases:"info,show" help:"Get a teacher"`
	Add    ClassroomTeachersAddCmd    `cmd:"" aliases:"create,new" help:"Add a teacher"`
	Remove ClassroomTeachersRemoveCmd `cmd:"" aliases:"delete,rm,del,remove" help:"Remove a teacher"`
}

type ClassroomTeachersListCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
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

	fetch := func(pageToken string) ([]*classroom.Teacher, string, error) {
		call := svc.Courses.Teachers.List(courseID).PageSize(c.Max).Context(ctx)
		if strings.TrimSpace(pageToken) != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			return nil, "", wrapClassroomError(err)
		}
		return resp.Teachers, resp.NextPageToken, nil
	}

	var teachers []*classroom.Teacher
	nextPageToken := ""
	if c.All {
		all, err := collectAllPages(c.Page, fetch)
		if err != nil {
			return err
		}
		teachers = all
	} else {
		var err error
		teachers, nextPageToken, err = fetch(c.Page)
		if err != nil {
			return err
		}
	}

	if outfmt.IsJSON(ctx) {
		if err := outfmt.WriteJSON(ctx, os.Stdout, map[string]any{
			"teachers":      teachers,
			"nextPageToken": nextPageToken,
		}); err != nil {
			return err
		}
		if len(teachers) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if len(teachers) == 0 {
		u.Err().Println("No teachers")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "USER_ID\tEMAIL\tNAME")
	for _, teacher := range teachers {
		if teacher == nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			sanitizeTab(teacher.UserId),
			sanitizeTab(profileEmail(teacher.Profile)),
			sanitizeTab(profileName(teacher.Profile)),
		)
	}
	printNextPageHint(u, nextPageToken)
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
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"teacher": teacher})
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
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	if err := dryRunExit(ctx, flags, "classroom.teachers.add", map[string]any{
		"course_id": courseID,
		"user_id":   userID,
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

	teacher := &classroom.Teacher{UserId: userID}
	created, err := svc.Courses.Teachers.Create(courseID, teacher).Context(ctx).Do()
	if err != nil {
		return wrapClassroomError(err)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(ctx, os.Stdout, map[string]any{"teacher": created})
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
	courseID := strings.TrimSpace(c.CourseID)
	userID := strings.TrimSpace(c.UserID)
	if courseID == "" {
		return usage("empty courseId")
	}
	if userID == "" {
		return usage("empty userId")
	}

	if err := confirmDestructive(ctx, flags, fmt.Sprintf("remove teacher %s from %s", userID, courseID)); err != nil {
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

	if _, err := svc.Courses.Teachers.Delete(courseID, userID).Context(ctx).Do(); err != nil {
		return wrapClassroomError(err)
	}

	return writeResult(ctx, u,
		kv("removed", true),
		kv("courseId", courseID),
		kv("userId", userID),
	)
}

type ClassroomRosterCmd struct {
	CourseID  string `arg:"" name:"courseId" help:"Course ID or alias"`
	Students  bool   `name:"students" help:"Include students"`
	Teachers  bool   `name:"teachers" help:"Include teachers"`
	Max       int64  `name:"max" aliases:"limit" help:"Max results (per role)" default:"100"`
	Page      string `name:"page" aliases:"cursor" help:"Page token (per role)"`
	All       bool   `name:"all" aliases:"all-pages,allpages" help:"Fetch all pages (per role)"`
	FailEmpty bool   `name:"fail-empty" aliases:"non-empty,require-results" help:"Exit with code 3 if no results"`
}

//nolint:gocyclo,cyclop // command orchestration across two role paths
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

	var students []*classroom.Student
	var teachers []*classroom.Teacher
	studentsNextPageToken := ""
	teachersNextPageToken := ""

	if includeStudents {
		fetch := func(pageToken string) ([]*classroom.Student, string, error) {
			call := svc.Courses.Students.List(courseID).PageSize(c.Max).Context(ctx)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}
			resp, callErr := call.Do()
			if callErr != nil {
				return nil, "", wrapClassroomError(callErr)
			}
			return resp.Students, resp.NextPageToken, nil
		}
		if c.All {
			all, collectErr := collectAllPages(c.Page, fetch)
			if collectErr != nil {
				return collectErr
			}
			students = all
		} else {
			students, studentsNextPageToken, err = fetch(c.Page)
			if err != nil {
				return err
			}
		}
	}
	if includeTeachers {
		fetch := func(pageToken string) ([]*classroom.Teacher, string, error) {
			call := svc.Courses.Teachers.List(courseID).PageSize(c.Max).Context(ctx)
			if strings.TrimSpace(pageToken) != "" {
				call = call.PageToken(pageToken)
			}
			resp, callErr := call.Do()
			if callErr != nil {
				return nil, "", wrapClassroomError(callErr)
			}
			return resp.Teachers, resp.NextPageToken, nil
		}
		if c.All {
			all, collectErr := collectAllPages(c.Page, fetch)
			if collectErr != nil {
				return collectErr
			}
			teachers = all
		} else {
			teachers, teachersNextPageToken, err = fetch(c.Page)
			if err != nil {
				return err
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		payload := map[string]any{"courseId": courseID}
		if includeStudents {
			payload["students"] = students
			payload["studentsNextPageToken"] = studentsNextPageToken
		}
		if includeTeachers {
			payload["teachers"] = teachers
			payload["teachersNextPageToken"] = teachersNextPageToken
		}
		if err := outfmt.WriteJSON(ctx, os.Stdout, payload); err != nil {
			return err
		}
		if includeStudents && includeTeachers && len(students) == 0 && len(teachers) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		if includeStudents && !includeTeachers && len(students) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		if includeTeachers && !includeStudents && len(teachers) == 0 {
			return failEmptyExit(c.FailEmpty)
		}
		return nil
	}

	if includeStudents && includeTeachers && len(students) == 0 && len(teachers) == 0 {
		u.Err().Println("No roster entries")
		return failEmptyExit(c.FailEmpty)
	}
	if includeStudents && !includeTeachers && len(students) == 0 {
		u.Err().Println("No students")
		return failEmptyExit(c.FailEmpty)
	}
	if includeTeachers && !includeStudents && len(teachers) == 0 {
		u.Err().Println("No teachers")
		return failEmptyExit(c.FailEmpty)
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "ROLE\tUSER_ID\tEMAIL\tNAME")
	if includeTeachers {
		for _, teacher := range teachers {
			if teacher == nil {
				continue
			}
			fmt.Fprintf(w, "teacher\t%s\t%s\t%s\n",
				sanitizeTab(teacher.UserId),
				sanitizeTab(profileEmail(teacher.Profile)),
				sanitizeTab(profileName(teacher.Profile)),
			)
		}
		if teachersNextPageToken != "" {
			u.Err().Printf("# Next teachers page: --page %s", teachersNextPageToken)
		}
	}
	if includeStudents {
		for _, student := range students {
			if student == nil {
				continue
			}
			fmt.Fprintf(w, "student\t%s\t%s\t%s\n",
				sanitizeTab(student.UserId),
				sanitizeTab(profileEmail(student.Profile)),
				sanitizeTab(profileName(student.Profile)),
			)
		}
		if studentsNextPageToken != "" {
			u.Err().Printf("# Next students page: --page %s", studentsNextPageToken)
		}
	}
	return nil
}
