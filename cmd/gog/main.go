package main

import (
	"os"

	"github.com/steipete/gogcli/internal/cmd"
	_ "github.com/steipete/gogcli/internal/tzembed" // Embed IANA timezone database for Windows support
)

func main() {
	if err := cmd.Execute(os.Args[1:]); err != nil {
		os.Exit(cmd.ExitCode(err))
	}
}
