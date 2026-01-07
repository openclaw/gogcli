package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

func helpPrinter(options kong.HelpOptions, ctx *kong.Context) error {
	origStdout := ctx.Stdout
	origStderr := ctx.Stderr

	width := guessColumns(origStdout)

	oldCols, hadCols := os.LookupEnv("COLUMNS")
	_ = os.Setenv("COLUMNS", strconv.Itoa(width))
	defer func() {
		if hadCols {
			_ = os.Setenv("COLUMNS", oldCols)
		} else {
			_ = os.Unsetenv("COLUMNS")
		}
	}()

	buf := bytes.NewBuffer(nil)
	ctx.Stdout = buf
	ctx.Stderr = origStderr
	defer func() { ctx.Stdout = origStdout }()

	if err := kong.DefaultHelpPrinter(options, ctx); err != nil {
		return err
	}

	out := rewriteCommandSummaries(buf.String(), ctx.Selected())
	out = injectBuildLine(out)
	out = colorizeHelp(out, helpProfile(origStdout, helpColorMode(ctx.Args)))
	_, err := io.WriteString(origStdout, out)
	return err
}

func injectBuildLine(out string) string {
	v := strings.TrimSpace(version)
	if v == "" {
		v = "dev"
	}
	c := strings.TrimSpace(commit)
	line := fmt.Sprintf("Build: %s", v)
	if c != "" {
		line = fmt.Sprintf("%s (%s)", line, c)
	}

	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "Usage:") {
			if i+1 < len(lines) && lines[i+1] == line {
				return out
			}
			outLines := make([]string, 0, len(lines)+1)
			outLines = append(outLines, lines[:i+1]...)
			outLines = append(outLines, line)
			outLines = append(outLines, lines[i+1:]...)
			return strings.Join(outLines, "\n")
		}
	}
	return out
}

func helpColorMode(args []string) string {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("GOG_COLOR"))); v != "" {
		return v
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--plain" || a == "--json" {
			return "never"
		}
		if a == "--color" && i+1 < len(args) {
			return strings.ToLower(strings.TrimSpace(args[i+1]))
		}
		if strings.HasPrefix(a, "--color=") {
			return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(a, "--color=")))
		}
	}
	return "auto"
}

func helpProfile(stdout io.Writer, mode string) termenv.Profile {
	if termenv.EnvNoColor() {
		return termenv.Ascii
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "never":
		return termenv.Ascii
	case "always":
		return termenv.TrueColor
	default:
		o := termenv.NewOutput(stdout, termenv.WithProfile(termenv.EnvColorProfile()))
		return o.Profile
	}
}

func colorizeHelp(out string, profile termenv.Profile) string {
	if profile == termenv.Ascii {
		return out
	}
	heading := func(s string) string {
		return termenv.String(s).Foreground(profile.Color("#60a5fa")).Bold().String()
	}
	section := func(s string) string {
		return termenv.String(s).Foreground(profile.Color("#a78bfa")).Bold().String()
	}
	group := func(s string) string {
		return termenv.String(s).Foreground(profile.Color("#34d399")).Bold().String()
	}
	cmdName := func(s string) string {
		return termenv.String(s).Foreground(profile.Color("#38bdf8")).Bold().String()
	}
	dim := func(s string) string {
		return termenv.String(s).Foreground(profile.Color("#9ca3af")).String()
	}

	inCommands := false
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if line == "Commands:" {
			inCommands = true
		}
		switch {
		case strings.HasPrefix(line, "Usage:"):
			lines[i] = heading("Usage:") + strings.TrimPrefix(line, "Usage:")
		case line == "Flags:":
			lines[i] = section(line)
		case line == "Commands:":
			lines[i] = section(line)
		case line == "Arguments:":
			lines[i] = section(line)
		case strings.HasPrefix(line, "Build:") || line == "Config:":
			lines[i] = section(line)
		case line == "Read" || line == "Write" || line == "Organize" || line == "Admin":
			lines[i] = group(line)
		case inCommands && strings.HasPrefix(line, "  ") && (len(line) < 3 || line[2] != ' '):
			lines[i] = colorizeCommandSummaryLine(line, cmdName, dim)
		case inCommands && strings.HasPrefix(line, "    ") && strings.TrimSpace(line) != "":
			lines[i] = "    " + dim(strings.TrimPrefix(line, "    "))
		}
	}
	return strings.Join(lines, "\n")
}

func colorizeCommandSummaryLine(line string, cmdName func(string) string, dim func(string) string) string {
	if !strings.HasPrefix(line, "  ") {
		return line
	}
	rest := strings.TrimPrefix(line, "  ")
	if rest == "" {
		return line
	}
	name, tail, _ := strings.Cut(rest, " ")
	if name == "" {
		return line
	}

	styled := cmdName(name)
	if tail == "" {
		return "  " + styled
	}

	// Keep placeholders readable but lower-contrast.
	tail = strings.ReplaceAll(tail, "<", dim("<"))
	tail = strings.ReplaceAll(tail, ">", dim(">"))
	tail = strings.ReplaceAll(tail, "[flags]", dim("[flags]"))
	return "  " + styled + " " + tail
}

func rewriteCommandSummaries(out string, selected *kong.Node) string {
	if selected == nil {
		return out
	}
	prefix := selected.Path() + " "
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if strings.HasPrefix(trimmed, prefix) && strings.HasPrefix(line, "  ") {
			indent := line[:len(line)-len(trimmed)]
			lines[i] = indent + strings.TrimPrefix(trimmed, prefix)
		}
	}
	return strings.Join(lines, "\n")
}

func guessColumns(w io.Writer) int {
	if colsStr := os.Getenv("COLUMNS"); colsStr != "" {
		if cols, err := strconv.Atoi(colsStr); err == nil {
			return cols
		}
	}
	f, ok := w.(*os.File)
	if !ok {
		return 80
	}

	width, _, err := term.GetSize(int(f.Fd()))
	if err == nil && width > 0 {
		return width
	}
	return 80
}
