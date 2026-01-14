package cmd

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

// CalendarProposeTimeCmd generates a browser URL for proposing a new meeting time.
// This is a workaround for a Google Calendar API limitation (since 2018).
type CalendarProposeTimeCmd struct {
	CalendarID string `arg:"" name:"calendarId" help:"Calendar ID"`
	EventID    string `arg:"" name:"eventId" help:"Event ID"`
	Open       bool   `name:"open" help:"Open the URL in browser automatically"`
	Decline    bool   `name:"decline" help:"Also decline the event (notifies organizer)"`
	Comment    string `name:"comment" help:"Comment to include with decline (implies --decline)"`
}
