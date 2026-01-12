package cmd

import (
	"github.com/steipete/gogcli/internal/googleapi"
)

var newClassroomService = googleapi.NewClassroom

var _ = newClassroomService

type ClassroomCmd struct {
	Courses ClassroomCoursesCmd `cmd:"" name:"courses" help:"Manage courses" default:"withargs"`
}

type ClassroomCoursesCmd struct{}
