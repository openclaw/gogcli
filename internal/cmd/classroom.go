package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/googleapi"

	intgoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newClassroomService = intgoogleapi.NewClassroom

type ClassroomCmd struct {
	Courses ClassroomCoursesCmd `cmd:"" name:"courses" help:"Manage courses" default:"withargs"`
}

type ClassroomCoursesCmd struct {
	List   ClassroomCoursesListCmd   `cmd:"" default:"withargs" help:"List courses"`
	Get    ClassroomCoursesGetCmd    `cmd:"" name:"get" help:"Get course details"`
	Create ClassroomCoursesCreateCmd `cmd:"" name:"create" help:"Create a new course"`
	Update ClassroomCoursesUpdateCmd `cmd:"" name:"update" help:"Update course details"`
}

type ClassroomCoursesListCmd struct {
	Role string `name:"role" help:"Filter by role: teacher or student"`
	Max  int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
}

type ClassroomCoursesGetCmd struct {
	CourseID string `arg:"" name:"course-id" help:"Course ID to get details for"`
}

type ClassroomCoursesCreateCmd struct {
	Name        string `name:"name" required:"" help:"Course name (required)"`
	Section     string `name:"section" help:"Course section (e.g., 'Period 1')"`
	Description string `name:"description" help:"Course description"`
	Room        string `name:"room" help:"Room location"`
}

type ClassroomCoursesUpdateCmd struct {
	CourseID    string `arg:"" name:"course-id" help:"Course ID to update"`
	Name        string `name:"name" help:"New course name"`
	Section     string `name:"section" help:"New course section"`
	Description string `name:"description" help:"New course description"`
	Room        string `name:"room" help:"New room location"`
	State       string `name:"state" help:"New course state (ACTIVE, ARCHIVED, PROVISIONED, DECLINED, SUSPENDED)"`
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

func (c *ClassroomCoursesGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	course, err := svc.Courses.Get(c.CourseID).Do()
	if err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == http.StatusNotFound {
			return usagef("course not found: %s", c.CourseID)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, course)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintf(w, "Name:\t%s\n", course.Name)
	fmt.Fprintf(w, "Section:\t%s\n", orDash(course.Section))
	fmt.Fprintf(w, "Description:\t%s\n", orDash(course.Description))
	fmt.Fprintf(w, "Room:\t%s\n", orDash(course.Room))
	fmt.Fprintf(w, "Owner ID:\t%s\n", course.OwnerId)
	fmt.Fprintf(w, "State:\t%s\n", course.CourseState)
	fmt.Fprintf(w, "Creation Time:\t%s\n", course.CreationTime)
	fmt.Fprintf(w, "Enrollment Code:\t%s\n", orDash(course.EnrollmentCode))
	fmt.Fprintf(w, "Web URL:\t%s\n", course.AlternateLink)

	return nil
}

func (c *ClassroomCoursesCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	course := &classroom.Course{
		Name:        c.Name,
		Section:     c.Section,
		Description: c.Description,
		Room:        c.Room,
		OwnerId:     "me",
	}

	created, err := svc.Courses.Create(course).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	fmt.Printf("Created course: %s\n", created.Id)
	return nil
}

func (c *ClassroomCoursesUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	course := &classroom.Course{}
	var mask []string

	if c.Name != "" {
		course.Name = c.Name
		mask = append(mask, "name")
	}
	if c.Section != "" {
		course.Section = c.Section
		mask = append(mask, "section")
	}
	if c.Description != "" {
		course.Description = c.Description
		mask = append(mask, "description")
	}
	if c.Room != "" {
		course.Room = c.Room
		mask = append(mask, "room")
	}
	if c.State != "" {
		switch c.State {
		case "ACTIVE", "ARCHIVED", "PROVISIONED", "DECLINED", "SUSPENDED":
			course.CourseState = c.State
			mask = append(mask, "courseState")
		default:
			return usagef("invalid state: %s (must be ACTIVE, ARCHIVED, PROVISIONED, DECLINED, or SUSPENDED)", c.State)
		}
	}

	if len(mask) == 0 {
		return usage("no fields to update")
	}

	updateMask := strings.Join(mask, ",")
	updated, err := svc.Courses.Patch(c.CourseID, course).UpdateMask(updateMask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	fmt.Printf("Updated course: %s\n", updated.Id)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
