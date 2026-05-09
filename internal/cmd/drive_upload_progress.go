package cmd

import (
	"context"
	"io"

	"github.com/steipete/gogcli/internal/outfmt"
	"github.com/steipete/gogcli/internal/ui"
)

const (
	driveUploadProgressMinBytes = 1 << 20
	driveUploadProgressStep     = 5
)

type driveUploadProgressReader struct {
	reader  io.Reader
	size    int64
	read    int64
	nextPct int64
	logf    func(string, ...any)
	done    bool
}

func driveUploadReader(ctx context.Context, reader io.Reader, opts driveUploadOptions) io.Reader {
	if outfmt.IsJSON(ctx) || opts.size < driveUploadProgressMinBytes {
		return reader
	}
	u := ui.FromContext(ctx)
	if u == nil || u.Err() == nil {
		return reader
	}
	return &driveUploadProgressReader{
		reader:  reader,
		size:    opts.size,
		nextPct: driveUploadProgressStep,
		logf:    u.Err().Printf,
	}
}

func (r *driveUploadProgressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.read += int64(n)
		r.report(false)
	}
	if err == io.EOF {
		r.report(true)
	}
	return n, err
}

func (r *driveUploadProgressReader) report(final bool) {
	if r.size <= 0 || r.logf == nil {
		return
	}
	if final && r.done {
		return
	}
	pct := r.read * 100 / r.size
	if pct > 100 {
		pct = 100
	}
	if !final && pct < r.nextPct {
		return
	}
	if final && r.read < r.size {
		return
	}

	r.logf("upload: %s / %s (%d%%)", formatDriveSize(r.read), formatDriveSize(r.size), pct)
	if final || pct >= 100 {
		r.done = true
	}
	for r.nextPct <= pct {
		r.nextPct += driveUploadProgressStep
	}
}
