package cmd

type ResourcesCmd struct {
	Buildings ResourcesBuildingsCmd `cmd:"" name:"buildings" help:"Manage resource buildings"`
	Calendars ResourcesCalendarsCmd `cmd:"" name:"calendars" help:"Manage resource calendars"`
	Features  ResourcesFeaturesCmd  `cmd:"" name:"features" help:"Manage resource features"`
}
