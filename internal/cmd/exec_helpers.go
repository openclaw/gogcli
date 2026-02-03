package cmd

import (
	"context"
	"fmt"
	"strings"
)

var executeSubcommand = runSubcommand

func runSubcommand(ctx context.Context, flags *RootFlags, args []string) error {
	parser, cli, err := newParser(helpDescription())
	if err != nil {
		return err
	}

	kctx, err := parser.Parse(args)
	if err != nil {
		if parsedErr := wrapParseError(err); parsedErr != nil {
			return parsedErr
		}
		return err
	}

	if ctx != nil {
		kctx.BindTo(ctx, (*context.Context)(nil))
	}
	if flags != nil {
		cli.RootFlags = *flags
		kctx.Bind(flags)
	}

	if err := enforceEnabledCommands(kctx, cli.EnableCommands); err != nil {
		return err
	}

	return kctx.Run()
}

func splitCommandLine(line string) ([]string, error) {
	args := []string{}
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if buf.Len() > 0 {
			args = append(args, buf.String())
			buf.Reset()
		}
	}

	for _, r := range line {
		switch {
		case escaped:
			buf.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			flush()
		default:
			buf.WriteRune(r)
		}
	}

	if escaped {
		return nil, fmt.Errorf("unterminated escape")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote")
	}
	flush()
	return args, nil
}
