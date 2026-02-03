package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/steipete/gogcli/internal/ui"
)

type BatchCmd struct {
	File     string `arg:"" name:"file" help:"Batch file"`
	Parallel int    `name:"parallel" help:"Number of commands to run in parallel" default:"1"`
	DryRun   bool   `name:"dry-run" help:"Preview commands without executing"`
}

func (c *BatchCmd) Run(ctx context.Context, flags *RootFlags) error {
	u := ui.FromContext(ctx)
	if strings.TrimSpace(c.File) == "" {
		return usage("file is required")
	}

	lines, err := readBatchLines(c.File)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		return fmt.Errorf("no commands found")
	}

	// Handle dry-run mode
	if c.DryRun {
		for _, task := range lines {
			if u != nil {
				u.Err().Printf("[dry-run] line %d: %s\n", task.Line, strings.Join(task.Args, " "))
			}
		}
		if u != nil {
			u.Err().Printf("Batch preview: total=%d (no commands executed)\n", len(lines))
		}
		return nil
	}

	parallel := c.Parallel
	if parallel < 1 {
		parallel = 1
	}

	tasks := make(chan batchTask)
	results := make(chan error, len(lines))
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for task := range tasks {
			if err := executeSubcommand(ctx, flags, task.Args); err != nil {
				results <- fmt.Errorf("line %d: %w", task.Line, err)
				continue
			}
			results <- nil
		}
	}

	for i := 0; i < parallel; i++ {
		wg.Add(1)
		go worker()
	}

	for _, task := range lines {
		tasks <- task
	}
	close(tasks)
	wg.Wait()
	close(results)

	failed := 0
	for err := range results {
		if err != nil {
			failed++
			if u != nil {
				u.Err().Error(err.Error())
			}
		}
	}

	if u != nil {
		u.Err().Printf("Batch complete: total=%d failed=%d\n", len(lines), failed)
	}
	if failed > 0 {
		return fmt.Errorf("%d commands failed", failed)
	}
	return nil
}

type batchTask struct {
	Line int
	Args []string
}

func readBatchLines(path string) ([]batchTask, error) {
	var scanner *bufio.Scanner
	if strings.TrimSpace(path) == "-" {
		scanner = bufio.NewScanner(os.Stdin)
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open batch file: %w", err)
		}
		defer f.Close()
		scanner = bufio.NewScanner(f)
	}

	lines := []batchTask{}
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		args, err := splitCommandLine(line)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if len(args) == 0 {
			continue
		}
		lines = append(lines, batchTask{Line: lineNo, Args: args})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read batch file: %w", err)
	}

	return lines, nil
}
