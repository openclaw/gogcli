package cmd

import "testing"

func TestParseEnabledCommands(t *testing.T) {
	allow := parseEnabledCommands("calendar, tasks ,Gmail")
	if !allow["calendar"] || !allow["tasks"] || !allow["gmail"] {
		t.Fatalf("unexpected allow map: %#v", allow)
	}
}

func TestCommandPathMatches(t *testing.T) {
	rules := parseEnabledCommands("gmail.search,config.no-send,calendar")
	cases := []struct {
		name string
		path []string
		want bool
	}{
		{name: "exact subcommand", path: []string{"gmail", "search"}, want: true},
		{name: "subcommand child", path: []string{"config", "no-send", "list"}, want: true},
		{name: "parent", path: []string{"calendar", "events"}, want: true},
		{name: "sibling blocked", path: []string{"gmail", "send"}, want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandPathMatches(rules, tt.path); got != tt.want {
				t.Fatalf("commandPathMatches(%v) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
