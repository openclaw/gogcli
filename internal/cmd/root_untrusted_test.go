package cmd

import "testing"

func TestWrapUntrustedFlag(t *testing.T) {
	t.Parallel()

	parser, cli, err := newParser(helpDescription())
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	if _, err := parser.Parse([]string{"--wrap-untrusted", "version"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.WrapUntrusted {
		t.Fatalf("expected --wrap-untrusted to enable wrapping")
	}
}

func TestWrapUntrustedEnvDefault(t *testing.T) {
	t.Setenv("GOG_WRAP_UNTRUSTED", "1")

	parser, cli, err := newParser(helpDescription())
	if err != nil {
		t.Fatalf("newParser: %v", err)
	}
	if _, err := parser.Parse([]string{"version"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cli.WrapUntrusted {
		t.Fatalf("expected GOG_WRAP_UNTRUSTED to enable wrapping")
	}
}
