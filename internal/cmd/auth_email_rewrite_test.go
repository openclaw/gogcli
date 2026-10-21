package cmd

import (
	"io"
	"reflect"
	"testing"
)

func TestRewriteAuthEmailArgs(t *testing.T) {
	t.Parallel()

	model := desirePathModel(t)
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "email",
			args: []string{"auth", "you@example.com"},
			want: []string{"auth", "add", "you@example.com"},
		},
		{
			name: "trailing flags",
			args: []string{"auth", "you@example.com", "--services", "gmail", "--manual"},
			want: []string{"auth", "add", "you@example.com", "--services", "gmail", "--manual"},
		},
		{
			name: "global boolean before auth",
			args: []string{"--json", "auth", "you@example.com"},
			want: []string{"--json", "auth", "add", "you@example.com"},
		},
		{
			name: "global value before auth",
			args: []string{"--account", "other@example.com", "auth", "you@example.com"},
			want: []string{"--account", "other@example.com", "auth", "add", "you@example.com"},
		},
		{
			name: "short global value before auth",
			args: []string{"-a", "other@example.com", "auth", "you@example.com"},
			want: []string{"-a", "other@example.com", "auth", "add", "you@example.com"},
		},
		{
			name: "inline global value",
			args: []string{"--account=other@example.com", "auth", "you@example.com"},
			want: []string{"--account=other@example.com", "auth", "add", "you@example.com"},
		},
		{
			name: "flags between auth and email",
			args: []string{"auth", "--json", "--account", "other@example.com", "you@example.com"},
			want: []string{"auth", "add", "--json", "--account", "other@example.com", "you@example.com"},
		},
		{
			name: "global flag alias",
			args: []string{"auth", "--pick", "email", "you@example.com"},
			want: []string{"auth", "add", "--pick", "email", "you@example.com"},
		},
		{
			name: "trailing args preserved",
			args: []string{"auth", "you@example.com", "extra", "--", "--manual"},
			want: []string{"auth", "add", "you@example.com", "extra", "--", "--manual"},
		},
		{name: "global email value with list", args: []string{"--account", "other@example.com", "auth", "list"}},
		{name: "bare auth", args: []string{"auth"}},
		{name: "typo", args: []string{"auth", "lsit"}},
		{name: "typo before email", args: []string{"auth", "lsit", "you@example.com"}},
		{name: "explicit add", args: []string{"auth", "add", "you@example.com"}},
		{name: "alias set", args: []string{"auth", "alias", "set", "work", "you@example.com"}},
		{name: "remove", args: []string{"auth", "remove", "you@example.com"}},
		{name: "command alias", args: []string{"auth", "login", "you@example.com"}},
		{name: "other command", args: []string{"gmail", "send", "--to", "you@example.com"}},
		{name: "nested auth token", args: []string{"gmail", "auth", "you@example.com"}},
		{name: "auth is flag value", args: []string{"--account", "auth", "you@example.com"}},
		{name: "only flag value after auth", args: []string{"auth", "--account", "other@example.com"}},
		{name: "delimiter before auth", args: []string{"--", "auth", "you@example.com"}},
		{name: "delimiter after auth", args: []string{"auth", "--", "you@example.com"}},
		{name: "delimiter after flag", args: []string{"auth", "--json", "--", "you@example.com"}},
		{name: "no args"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			want := tt.want
			if want == nil {
				want = tt.args
			}
			before := append([]string(nil), tt.args...)
			if got := rewriteAuthEmailArgs(model, tt.args); !reflect.DeepEqual(got, want) {
				t.Fatalf("rewriteAuthEmailArgs(%v) = %v, want %v", tt.args, got, want)
			}
			if !reflect.DeepEqual(tt.args, before) {
				t.Fatalf("input mutated: got %v, want %v", tt.args, before)
			}
		})
	}
}

func TestAuthEmailShorthandParse(t *testing.T) {
	parser, cli, err := newParserWithWriters("test parser", io.Discard, io.Discard)
	if err != nil {
		t.Fatalf("newParserWithWriters: %v", err)
	}
	args := rewriteAuthEmailArgs(parser.Model, []string{"auth", "you@example.com"})
	ctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := ctx.Command(); got != "auth add <email>" {
		t.Fatalf("command = %q, want auth add <email>", got)
	}
	if got := cli.Auth.Add.Email; got != "you@example.com" {
		t.Fatalf("email = %q, want you@example.com", got)
	}
}
