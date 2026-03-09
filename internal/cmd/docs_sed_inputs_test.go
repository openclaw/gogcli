package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectExpressions_Positional(t *testing.T) {
	cmd := &DocsSedCmd{Expression: "s/foo/bar/"}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 1 || exprs[0] != "s/foo/bar/" {
		t.Errorf("got %v, want [s/foo/bar/]", exprs)
	}
}

func TestCollectExpressions_MultipleE(t *testing.T) {
	cmd := &DocsSedCmd{
		Expressions: []string{"s/foo/bar/", "s/baz/qux/g"},
	}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2", len(exprs))
	}
	if exprs[0] != "s/foo/bar/" || exprs[1] != "s/baz/qux/g" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_PositionalPlusE(t *testing.T) {
	cmd := &DocsSedCmd{
		Expression:  "s/first/one/",
		Expressions: []string{"s/second/two/"},
	}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 || exprs[0] != "s/first/one/" || exprs[1] != "s/second/two/" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edits.sed")
	content := "# Comment line\ns/foo/bar/\n\ns/baz/**qux**/g\n# Another comment\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &DocsSedCmd{File: path}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2: %v", len(exprs), exprs)
	}
	if exprs[0] != "s/foo/bar/" || exprs[1] != "s/baz/**qux**/g" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_FilePlusE(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edits.sed")
	if err := os.WriteFile(path, []byte("s/from-file/yes/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &DocsSedCmd{
		Expressions: []string{"s/from-flag/yes/"},
		File:        path,
	}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2", len(exprs))
	}
	if exprs[0] != "s/from-flag/yes/" || exprs[1] != "s/from-file/yes/" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_Stdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		_, _ = w.WriteString("# comment\ns/from-stdin/yes/\ns/also-stdin/**bold**/g\n")
		_ = w.Close()
	}()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cmd := &DocsSedCmd{}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 2 {
		t.Fatalf("got %d exprs, want 2: %v", len(exprs), exprs)
	}
	if exprs[0] != "s/from-stdin/yes/" || exprs[1] != "s/also-stdin/**bold**/g" {
		t.Errorf("got %v", exprs)
	}
}

func TestCollectExpressions_StdinIgnoredWhenEProvided(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_, _ = w.WriteString("s/should-not-appear/nope/\n")
		_ = w.Close()
	}()

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = oldStdin }()

	cmd := &DocsSedCmd{Expressions: []string{"s/from-flag/yes/"}}
	exprs, err := cmd.collectExpressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(exprs) != 1 || exprs[0] != "s/from-flag/yes/" {
		t.Errorf("got %v, want [s/from-flag/yes/]", exprs)
	}
}

func TestCollectExpressions_NoInput(t *testing.T) {
	cmd := &DocsSedCmd{}
	_, err := cmd.collectExpressions()
	if err == nil {
		t.Error("expected error for no expressions")
	}
}

func TestCollectExpressions_FileMissing(t *testing.T) {
	cmd := &DocsSedCmd{File: "/nonexistent/file.sed"}
	_, err := cmd.collectExpressions()
	if err == nil {
		t.Error("expected error for missing file")
	}
}
