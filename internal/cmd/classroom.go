package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/api/classroom/v1"
	"google.golang.org/api/googleapi"

	intgoogleapi "github.com/steipete/gogcli/internal/googleapi"
	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

var newClassroomService = intgoogleapi.NewClassroom

type ClassroomCmd struct {
	Courses ClassroomCoursesCmd `cmd:"" name:"courses" help:"Manage courses" default:"withargs"`
	Roster  ClassroomRosterCmd  `cmd:"" name:"roster" help:"Manage course roster (students and teachers)"`
	Work    ClassroomWorkCmd    `cmd:"" name:"work" help:"Manage coursework (assignments and materials)"`
}

type ClassroomWorkCmd struct {
	List   ClassroomWorkListCmd   `cmd:"" default:"withargs" help:"List coursework"`
	Get    ClassroomWorkGetCmd    `cmd:"" name:"get" help:"Get coursework details"`
	Create ClassroomWorkCreateCmd `cmd:"" name:"create" help:"Create new coursework"`
	Update ClassroomWorkUpdateCmd `cmd:"" name:"update" help:"Update coursework"`
	Delete ClassroomWorkDeleteCmd `cmd:"" name:"delete" help:"Delete coursework"`
}

type (
	ClassroomWorkListCmd struct {
		CourseID string `arg:"" name:"course-id" help:"Course ID" required:""`
		Type     string `name:"type" help:"Filter by type (assignment or material)"`
		Max      int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
	}
	ClassroomWorkGetCmd struct {
		CourseID string `arg:"" name:"course-id" help:"Course ID" required:""`
		WorkID   string `arg:"" name:"work-id" help:"CourseWork ID" required:""`
	}
	ClassroomWorkCreateCmd struct {
		CourseID    string `arg:"" name:"course-id" help:"Course ID" required:""`
		Title       string `name:"title" required:"" help:"Coursework title"`
		Description string `name:"description" help:"Description"`
		Due         string `name:"due" help:"Due date (RFC3339, date, relative)"`
		Points      int64  `name:"points" help:"Maximum points"`
		TopicId     string `name:"topic" help:"Topic ID"`
		State       string `name:"state" enum:"PUBLISHED,DRAFT" default:"PUBLISHED" help:"State (PUBLISHED or DRAFT)"`
		Type        string `name:"type" enum:"ASSIGNMENT,SHORT_ANSWER_QUESTION,MULTIPLE_CHOICE_QUESTION" default:"ASSIGNMENT" help:"Type of coursework"`
	}
	ClassroomWorkUpdateCmd struct {
		CourseID    string   `arg:"" name:"course-id" help:"Course ID" required:""`
		WorkID      string   `arg:"" name:"work-id" help:"CourseWork ID" required:""`
		Title       string   `name:"title" help:"New title"`
		Description string   `name:"description" help:"New description"`
		Due         string   `name:"due" help:"New due date (RFC3339, date, relative)"`
		Points      *float64 `name:"points" help:"New maximum points"`
		TopicId     string   `name:"topic" help:"New topic ID"`
		State       string   `name:"state" enum:"PUBLISHED,DRAFT" help:"New state (PUBLISHED or DRAFT)"`
	}
	ClassroomWorkDeleteCmd struct {
		CourseID string `arg:"" name:"course-id" help:"Course ID" required:""`
	}
)

type ClassroomRosterCmd struct {
	List   ClassroomRosterListCmd   `cmd:"" default:"withargs" help:"List roster members"`
	Add    ClassroomRosterAddCmd    `cmd:"" name:"add" help:"Add student or teacher"`
	Remove ClassroomRosterRemoveCmd `cmd:"" name:"remove" help:"Remove student or teacher"`
}

type (
	ClassroomRosterListCmd struct {
		CourseID string `arg:"" name:"course-id" help:"Course ID" required:""`
		Students bool   `name:"students" help:"List students only"`
		Teachers bool   `name:"teachers" help:"List teachers only"`
		Max      int64  `name:"max" aliases:"limit" help:"Max results per page" default:"100"`
	}
	ClassroomRosterAddCmd struct {
		CourseID string `arg:"" name:"course-id" help:"Course ID" required:""`
		Email    string `name:"email" required:"" help:"Email address of user to add"`
		Role     string `name:"role" required:"" enum:"student,teacher" help:"Role to assign (student or teacher)"`
	}
	ClassroomRosterRemoveCmd struct {
		CourseID string `arg:"" name:"course-id" help:"Course ID" required:""`
		Email    string `name:"email" required:"" help:"Email address of user to remove"`
		Role     string `name:"role" required:"" enum:"student,teacher" help:"Role to remove (student or teacher)"`
	}
)

type ClassroomCoursesCmd struct {
	List   ClassroomCoursesListCmd   `cmd:"" default:"withargs" help:"List courses"`
	Get    ClassroomCoursesGetCmd    `cmd:"" name:"get" help:"Get course details"`
	Create ClassroomCoursesCreateCmd `cmd:"" name:"create" help:"Create a new course"`
	Update ClassroomCoursesUpdateCmd `cmd:"" name:"update" help:"Update course details"`
	Delete ClassroomCoursesDeleteCmd `cmd:"" name:"delete" help:"Delete a course"`
	URL    ClassroomCoursesURLCmd    `cmd:"" name:"url" help:"Get course web URL"`
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

type ClassroomCoursesDeleteCmd struct {
	CourseID string `arg:"" name:"course-id" help:"Course ID to delete"`
}

type ClassroomCoursesURLCmd struct {
	CourseID string `arg:"" name:"course-id" help:"Course ID"`
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

func (c *ClassroomCoursesDeleteCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("delete course %s", c.CourseID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	if _, err := svc.Courses.Delete(c.CourseID).Do(); err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == http.StatusNotFound {
			return usagef("course not found: %s", c.CourseID)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"deleted": true,
			"id":      c.CourseID,
		})
	}

	fmt.Printf("Deleted course: %s\n", c.CourseID)
	return nil
}

func (c *ClassroomCoursesURLCmd) Run(ctx context.Context, flags *RootFlags) error {
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

	url := course.AlternateLink
	if url == "" {
		url = fmt.Sprintf("https://classroom.google.com/c/%s", c.CourseID)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]string{
			"url": url,
		})
	}

	fmt.Println(url)
	return nil
}

func (c *ClassroomWorkListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	if c.Type != "" && c.Type != "assignment" && c.Type != "material" {
		return usagef("invalid type: %q (must be 'assignment' or 'material')", c.Type)
	}

	var allWork []*classroom.CourseWork
	var allMaterials []*classroom.CourseWorkMaterial

	fetchWork := c.Type == "" || c.Type == "assignment"
	fetchMaterials := c.Type == "" || c.Type == "material"

	if fetchWork {
		pageToken := ""
		for {
			call := svc.Courses.CourseWork.List(c.CourseID).PageSize(c.Max)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}

			resp, err := call.Do()
			if err != nil {
				return fmt.Errorf("listing coursework: %w", err)
			}

			for _, cw := range resp.CourseWork {
				if c.Type == "assignment" && cw.WorkType != "ASSIGNMENT" {
					continue
				}
				allWork = append(allWork, cw)
			}

			pageToken = resp.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}

	if fetchMaterials {
		pageToken := ""
		for {
			call := svc.Courses.CourseWorkMaterials.List(c.CourseID).PageSize(c.Max)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}

			resp, err := call.Do()
			if err != nil {
				return fmt.Errorf("listing coursework materials: %w", err)
			}

			allMaterials = append(allMaterials, resp.CourseWorkMaterial...)

			pageToken = resp.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		result := map[string]any{
			"courseWork":          allWork,
			"courseWorkMaterials": allMaterials,
		}
		return outfmt.WriteJSON(os.Stdout, result)
	}

	if len(allWork) == 0 && len(allMaterials) == 0 {
		ui.FromContext(ctx).Err().Println("No coursework found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "WORK_ID\tTITLE\tTYPE\tSTATE\tDUE_DATE\tMAX_POINTS")

	for _, cw := range allWork {
		dueDate := formatDueDate(cw.DueDate, cw.DueTime)
		maxPoints := "-"
		if cw.MaxPoints != 0 {
			maxPoints = fmt.Sprintf("%g", cw.MaxPoints)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			cw.Id,
			cw.Title,
			cw.WorkType,
			cw.State,
			dueDate,
			maxPoints,
		)
	}

	for _, cm := range allMaterials {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			cm.Id,
			cm.Title,
			"MATERIAL",
			cm.State,
			"-",
			"-",
		)
	}

	return nil
}

func (c *ClassroomWorkGetCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	cw, err := svc.Courses.CourseWork.Get(c.CourseID, c.WorkID).Do()
	if err != nil {
		var gerr *googleapi.Error
		if errors.As(err, &gerr) && gerr.Code == http.StatusNotFound {
			return usagef("coursework not found: %s", c.WorkID)
		}
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, cw)
	}

	w, flush := tableWriter(ctx)
	defer flush()

	fmt.Fprintf(w, "Title:\t%s\n", cw.Title)
	fmt.Fprintf(w, "Description:\t%s\n", orDash(cw.Description))
	fmt.Fprintf(w, "State:\t%s\n", cw.State)
	fmt.Fprintf(w, "Type:\t%s\n", cw.WorkType)
	fmt.Fprintf(w, "Due:\t%s\n", formatDueDate(cw.DueDate, cw.DueTime))

	maxPoints := "-"
	if cw.MaxPoints != 0 {
		maxPoints = fmt.Sprintf("%g", cw.MaxPoints)
	}
	fmt.Fprintf(w, "Max Points:\t%s\n", maxPoints)

	fmt.Fprintf(w, "Topic ID:\t%s\n", orDash(cw.TopicId))
	fmt.Fprintf(w, "Creation Time:\t%s\n", cw.CreationTime)
	fmt.Fprintf(w, "Update Time:\t%s\n", cw.UpdateTime)
	fmt.Fprintf(w, "Web URL:\t%s\n", cw.AlternateLink)

	if len(cw.Materials) > 0 {
		fmt.Fprintln(w, "\nMaterials:")
		for _, m := range cw.Materials {
			if m.DriveFile != nil && m.DriveFile.DriveFile != nil {
				fmt.Fprintf(w, "  [Drive File] %s (%s)\n", m.DriveFile.DriveFile.Title, m.DriveFile.DriveFile.AlternateLink)
			}
			if m.YoutubeVideo != nil {
				fmt.Fprintf(w, "  [YouTube] %s (%s)\n", m.YoutubeVideo.Title, m.YoutubeVideo.AlternateLink)
			}
			if m.Link != nil {
				fmt.Fprintf(w, "  [Link] %s (%s)\n", m.Link.Title, m.Link.Url)
			}
			if m.Form != nil {
				fmt.Fprintf(w, "  [Form] %s (%s)\n", m.Form.Title, m.Form.FormUrl)
			}
		}
	}

	return nil
}

func (c *ClassroomWorkCreateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	cw := &classroom.CourseWork{
		Title:       c.Title,
		Description: c.Description,
		State:       c.State,
		WorkType:    c.Type,
		TopicId:     c.TopicId,
	}

	if c.Points > 0 {
		cw.MaxPoints = float64(c.Points)
	}

	if c.Due != "" {
		// Use local timezone for parsing
		var t time.Time
		t, err = parseTimeExpr(c.Due, time.Now(), time.Local)
		if err != nil {
			return fmt.Errorf("invalid due date: %w", err)
		}

		cw.DueDate = &classroom.Date{
			Year:  int64(t.Year()),
			Month: int64(t.Month()),
			Day:   int64(t.Day()),
		}
		cw.DueTime = &classroom.TimeOfDay{
			Hours:   int64(t.Hour()),
			Minutes: int64(t.Minute()),
		}
	}

	created, err := svc.Courses.CourseWork.Create(c.CourseID, cw).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	fmt.Printf("Created coursework: %s\n", created.Id)
	return nil
}

func (c *ClassroomWorkUpdateCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	cw := &classroom.CourseWork{}
	var mask []string

	if c.Title != "" {
		cw.Title = c.Title
		mask = append(mask, "title")
	}
	if c.Description != "" {
		cw.Description = c.Description
		mask = append(mask, "description")
	}
	if c.State != "" {
		cw.State = c.State
		mask = append(mask, "state")
	}
	if c.TopicId != "" {
		cw.TopicId = c.TopicId
		mask = append(mask, "topicId")
	}
	if c.Points != nil {
		cw.MaxPoints = *c.Points
		mask = append(mask, "maxPoints")
	}
	if c.Due != "" {
		var t time.Time
		t, err = parseTimeExpr(c.Due, time.Now(), time.Local)
		if err != nil {
			return fmt.Errorf("invalid due date: %w", err)
		}

		cw.DueDate = &classroom.Date{
			Year:  int64(t.Year()),
			Month: int64(t.Month()),
			Day:   int64(t.Day()),
		}
		cw.DueTime = &classroom.TimeOfDay{
			Hours:   int64(t.Hour()),
			Minutes: int64(t.Minute()),
		}
		mask = append(mask, "dueDate", "dueTime")
	}

	if len(mask) == 0 {
		return usage("no fields to update")
	}

	updateMask := strings.Join(mask, ",")
	updated, err := svc.Courses.CourseWork.Patch(c.CourseID, c.WorkID, cw).UpdateMask(updateMask).Do()
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, updated)
	}

	fmt.Printf("Updated coursework: %s\n", updated.Id)
	return nil
}

func formatDueDate(d *classroom.Date, t *classroom.TimeOfDay) string {
	if d == nil {
		return "-"
	}
	dateStr := fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
	if t != nil {
		return fmt.Sprintf("%s %02d:%02d", dateStr, t.Hours, t.Minutes)
	}
	return dateStr
}

func (c *ClassroomRosterListCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	fetchStudents := c.Students
	fetchTeachers := c.Teachers
	if !fetchStudents && !fetchTeachers {
		fetchStudents = true
		fetchTeachers = true
	}

	type rosterMember struct {
		UserID string `json:"userId"`
		Email  string `json:"email"`
		Name   string `json:"name"`
		Role   string `json:"role"`
	}

	var members []rosterMember

	if fetchTeachers {
		pageToken := ""
		for {
			call := svc.Courses.Teachers.List(c.CourseID).PageSize(c.Max)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}

			resp, err := call.Do()
			if err != nil {
				return fmt.Errorf("listing teachers: %w", err)
			}

			for _, teacher := range resp.Teachers {
				email := ""
				name := ""
				if teacher.Profile != nil {
					email = teacher.Profile.EmailAddress
					if teacher.Profile.Name != nil {
						name = teacher.Profile.Name.FullName
					}
				}
				members = append(members, rosterMember{
					UserID: teacher.UserId,
					Email:  email,
					Name:   name,
					Role:   "TEACHER",
				})
			}

			pageToken = resp.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}

	if fetchStudents {
		pageToken := ""
		for {
			call := svc.Courses.Students.List(c.CourseID).PageSize(c.Max)
			if pageToken != "" {
				call = call.PageToken(pageToken)
			}

			resp, err := call.Do()
			if err != nil {
				return fmt.Errorf("listing students: %w", err)
			}

			for _, student := range resp.Students {
				email := ""
				name := ""
				if student.Profile != nil {
					email = student.Profile.EmailAddress
					if student.Profile.Name != nil {
						name = student.Profile.Name.FullName
					}
				}
				members = append(members, rosterMember{
					UserID: student.UserId,
					Email:  email,
					Name:   name,
					Role:   "STUDENT",
				})
			}

			pageToken = resp.NextPageToken
			if pageToken == "" {
				break
			}
		}
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, members)
	}

	if len(members) == 0 {
		ui.FromContext(ctx).Err().Println("No roster members found")
		return nil
	}

	w, flush := tableWriter(ctx)
	defer flush()
	fmt.Fprintln(w, "USER_ID\tEMAIL\tNAME\tROLE")
	for _, m := range members {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			m.UserID,
			orDash(m.Email),
			orDash(m.Name),
			m.Role,
		)
	}
	return nil
}

func (c *ClassroomRosterAddCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	var created any

	switch c.Role {
	case "student":
		student := &classroom.Student{
			UserId: c.Email,
		}
		res, err := svc.Courses.Students.Create(c.CourseID, student).Do()
		if err != nil {
			return err
		}
		created = res
	case "teacher":
		teacher := &classroom.Teacher{
			UserId: c.Email,
		}
		res, err := svc.Courses.Teachers.Create(c.CourseID, teacher).Do()
		if err != nil {
			return err
		}
		created = res
	default:
		return usagef("invalid role: %s", c.Role)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, created)
	}

	fmt.Printf("Added %s: %s\n", c.Role, c.Email)
	return nil
}

func (c *ClassroomRosterRemoveCmd) Run(ctx context.Context, flags *RootFlags) error {
	account, err := requireAccount(flags)
	if err != nil {
		return err
	}

	if confirmErr := confirmDestructive(ctx, flags, fmt.Sprintf("remove %s %s from course %s", c.Role, c.Email, c.CourseID)); confirmErr != nil {
		return confirmErr
	}

	svc, err := newClassroomService(ctx, account)
	if err != nil {
		return err
	}

	switch c.Role {
	case "student":
		if _, err := svc.Courses.Students.Delete(c.CourseID, c.Email).Do(); err != nil {
			var gerr *googleapi.Error
			if errors.As(err, &gerr) && gerr.Code == http.StatusNotFound {
				return usagef("student not found in course: %s", c.Email)
			}
			return err
		}
	case "teacher":
		if _, err := svc.Courses.Teachers.Delete(c.CourseID, c.Email).Do(); err != nil {
			var gerr *googleapi.Error
			if errors.As(err, &gerr) && gerr.Code == http.StatusNotFound {
				return usagef("teacher not found in course: %s", c.Email)
			}
			return err
		}
	default:
		return usagef("invalid role: %s", c.Role)
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"removed": true,
			"role":    c.Role,
			"email":   c.Email,
		})
	}

	fmt.Printf("Removed %s: %s\n", c.Role, c.Email)
	return nil
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
