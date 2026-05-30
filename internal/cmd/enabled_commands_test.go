package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

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

func TestCommandPathMatchesExact(t *testing.T) {
	rules := parseEnabledCommands("gmail.search,config.no-send,calendar")
	cases := []struct {
		name string
		path []string
		want bool
	}{
		{name: "exact subcommand", path: []string{"gmail", "search"}, want: true},
		{name: "subcommand child blocked", path: []string{"config", "no-send", "list"}, want: false},
		{name: "parent does not allow child", path: []string{"calendar", "events"}, want: false},
		{name: "sibling blocked", path: []string{"gmail", "send"}, want: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandPathMatchesExact(rules, tt.path); got != tt.want {
				t.Fatalf("commandPathMatchesExact(%v) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCommandPathMatchesExactAll(t *testing.T) {
	rules := parseEnabledCommands("all")
	if !commandPathMatchesExact(rules, []string{"gmail", "send"}) {
		t.Fatal("commandPathMatchesExact(all, gmail send) = false, want true")
	}
}

func TestEnforceEnabledCommands(t *testing.T) {
	cases := []struct {
		name         string
		args         []string
		enabled      string
		enabledExact string
		wantErr      string
	}{
		{
			name:    "prefix allow permits child command",
			args:    []string{"gmail", "send"},
			enabled: "gmail",
		},
		{
			name:         "exact allow permits exact command",
			args:         []string{"gmail", "search", "test"},
			enabledExact: "gmail.search",
		},
		{
			name:         "exact allow blocks sibling command",
			args:         []string{"gmail", "send"},
			enabledExact: "gmail.search",
			wantErr:      `command "gmail send" is not enabled`,
		},
		{
			name:         "combined allowlists permit either match type",
			args:         []string{"calendar", "events"},
			enabled:      "drive",
			enabledExact: "calendar.events",
		},
		{
			name:         "neither allowlist matching returns usage error",
			args:         []string{"gmail", "send"},
			enabled:      "drive",
			enabledExact: "gmail.search",
			wantErr:      `set --enable-commands or --enable-commands-exact to allow it`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			kctx := parseEnabledCommandTestContext(t, tt.args...)
			err := enforceEnabledCommands(kctx, tt.enabled, tt.enabledExact)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("enforceEnabledCommands() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("enforceEnabledCommands() error = nil, want %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("enforceEnabledCommands() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestEnableCommandsExactEnvDefault(t *testing.T) {
	t.Setenv("GOG_ENABLE_COMMANDS", "")
	t.Setenv("GOG_ENABLE_COMMANDS_EXACT", "gmail.search")

	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser() error = %v", err)
	}
	kctx, err := parser.Parse([]string{"gmail", "send"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cli.EnableCommandsExact != "gmail.search" {
		t.Fatalf("EnableCommandsExact = %q, want %q", cli.EnableCommandsExact, "gmail.search")
	}
	err = enforceEnabledCommands(kctx, cli.EnableCommands, cli.EnableCommandsExact)
	if err == nil {
		t.Fatal("enforceEnabledCommands() error = nil, want exact env allowlist to block gmail send")
	}
	if !strings.Contains(err.Error(), `command "gmail send" is not enabled`) {
		t.Fatalf("enforceEnabledCommands() error = %q", err.Error())
	}
}

func parseEnabledCommandTestContext(t *testing.T, args ...string) *kong.Context {
	t.Helper()
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser() error = %v", err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("Parse(%v) error = %v", args, err)
	}
	return kctx
}
