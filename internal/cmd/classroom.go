package cmd

import "github.com/steipete/gogcli/internal/googleapi"

var newClassroomService = googleapi.NewClassroom

type ClassroomCmd struct {
	Courses         ClassroomCoursesCmd         `cmd:"" aliases:"course" help:"Courses"`
	Students        ClassroomStudentsCmd        `cmd:"" aliases:"student" help:"Course students"`
	Teachers        ClassroomTeachersCmd        `cmd:"" aliases:"teacher" help:"Course teachers"`
	Roster          ClassroomRosterCmd          `cmd:"" aliases:"members" help:"Course roster (students + teachers)"`
	Coursework      ClassroomCourseworkCmd      `cmd:"" name:"coursework" aliases:"work" help:"Coursework"`
	Materials       ClassroomMaterialsCmd       `cmd:"" name:"materials" aliases:"material" help:"Coursework materials"`
	Submissions     ClassroomSubmissionsCmd     `cmd:"" aliases:"submission" help:"Student submissions"`
	Announcements   ClassroomAnnouncementsCmd   `cmd:"" aliases:"announcement,ann" help:"Announcements"`
	Topics          ClassroomTopicsCmd          `cmd:"" aliases:"topic" help:"Topics"`
	Invitations     ClassroomInvitationsCmd     `cmd:"" aliases:"invitation,invites" help:"Invitations"`
	Guardians       ClassroomGuardiansCmd       `cmd:"" aliases:"guardian" help:"Guardians"`
	GuardianInvites ClassroomGuardianInvitesCmd `cmd:"" name:"guardian-invitations" aliases:"guardian-invites" help:"Guardian invitations"`
	Profile         ClassroomProfileCmd         `cmd:"" aliases:"me" help:"User profiles"`
}
