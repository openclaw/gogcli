package cmd

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/api/classroom/v1"

	"github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newClassroomService = googleapi.NewClassroom

type ClassroomCmd struct {
	Courses ClassroomCoursesCmd `cmd:"" name:"courses" help:"Manage courses" default:"withargs"`
}

type ClassroomCoursesCmd struct {
	List ClassroomCoursesListCmd `cmd:"" default:"withargs" help:"List courses"`
}

type ClassroomCoursesListCmd struct {
	Role string `name:"role" help:"Filter by role: teacher or student"`
	Max  int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
}

func (c *ClassroomCoursesListCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	var allCourses []*classroom.Course
	pageToken := ""

	for {
		call := svc.Courses.List().PageSize(c.Max)

		switch c.Role {
		case "teacher":
			call = call.TeacherId("me")
		case "student":
			call = call.StudentId("me")
		case "":
		default:
			return usage(fmt.Sprintf("invalid role: %q (must be 'teacher' or 'student')", c.Role))
		}

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return err
		}

		allCourses = append(allCourses, resp.Courses...)

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"courses": allCourses,
		})
	}

	if len(allCourses) == 0 {
		u.Err().Println("No courses found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "COURSE_ID\tNAME\tSECTION\tENROLLMENT_CODE\tSTATE")
	for _, course := range allCourses {
		section := course.Section
		if section == "" {
			section = "-"
		}
		enrollmentCode := course.EnrollmentCode
		if enrollmentCode == "" {
			enrollmentCode = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			course.Id,
			course.Name,
			section,
			enrollmentCode,
			course.CourseState,
		)
	}
	return nil
}
