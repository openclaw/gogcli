//nolint:wsl_v5 // The async queue state machine is clearer with compact lock/unlock blocks.
package backup

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultAsyncPushQueueLimit = 3
	asyncPushMaxAttempts       = 3
)

type asyncPushJob struct {
	sha     string
	message string
}

type asyncRepoPusher struct {
	cfg      Config
	progress func(format string, args ...any)

	mu      sync.Mutex
	cond    *sync.Cond
	queue   []asyncPushJob
	pushing bool
	err     error
}

var asyncPushers = struct {
	mu sync.Mutex
	m  map[string]*asyncRepoPusher
}{m: map[string]*asyncRepoPusher{}}

func enqueueAsyncPush(ctx context.Context, cfg Config, opts Options, sha string, message string) error {
	limit := opts.PushQueueLimit
	if limit <= 0 {
		limit = defaultAsyncPushQueueLimit
	}
	pusher := asyncPusherFor(ctx, cfg, opts.Progress)

	for {
		pusher.mu.Lock()
		if pusher.err != nil {
			err := pusher.err
			pusher.mu.Unlock()
			return err
		}
		if len(pusher.queue) < limit {
			pusher.queue = append(pusher.queue, asyncPushJob{sha: strings.TrimSpace(sha), message: message})
			pending := len(pusher.queue)
			pusher.cond.Signal()
			pusher.mu.Unlock()
			pusher.progressf("backup git push\tqueued\tsha=%s\tpending=%d\tmessage=%q", shortSHA(sha), pending, message)
			return nil
		}
		pending := len(pusher.queue)
		pusher.mu.Unlock()
		pusher.progressf("backup git push\tbackpressure\tpending=%d", pending)
		select {
		case <-ctx.Done():
			return fmt.Errorf("enqueue async backup push: %w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func waitAsyncPushes(ctx context.Context, repo string, progress func(format string, args ...any)) error {
	pusher := existingAsyncPusher(repo, progress)
	if pusher == nil {
		return nil
	}
	for {
		pusher.mu.Lock()
		if pusher.err != nil {
			err := pusher.err
			pusher.mu.Unlock()
			return err
		}
		done := len(pusher.queue) == 0 && !pusher.pushing
		pending := len(pusher.queue)
		pusher.mu.Unlock()
		if done {
			return nil
		}
		pusher.progressf("backup git push\tdrain\tpending=%d", pending)
		select {
		case <-ctx.Done():
			return fmt.Errorf("drain async backup pushes: %w", ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func asyncPusherActive(repo string) bool {
	pusher := existingAsyncPusher(repo, nil)
	if pusher == nil {
		return false
	}
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	return pusher.err != nil || pusher.pushing || len(pusher.queue) > 0
}

func asyncPusherFor(ctx context.Context, cfg Config, progress func(format string, args ...any)) *asyncRepoPusher {
	repo := strings.TrimSpace(cfg.Repo)
	asyncPushers.mu.Lock()
	defer asyncPushers.mu.Unlock()
	if pusher := asyncPushers.m[repo]; pusher != nil {
		if progress != nil {
			pusher.progress = progress
		}
		return pusher
	}
	pusher := &asyncRepoPusher{
		cfg:      cfg,
		progress: progress,
	}
	pusher.cond = sync.NewCond(&pusher.mu)
	asyncPushers.m[repo] = pusher
	go pusher.run(context.WithoutCancel(ctx))

	return pusher
}

func existingAsyncPusher(repo string, progress func(format string, args ...any)) *asyncRepoPusher {
	asyncPushers.mu.Lock()
	defer asyncPushers.mu.Unlock()
	pusher := asyncPushers.m[strings.TrimSpace(repo)]
	if pusher != nil && progress != nil {
		pusher.progress = progress
	}
	return pusher
}

func (p *asyncRepoPusher) run(ctx context.Context) {
	for {
		p.mu.Lock()
		for len(p.queue) == 0 {
			p.cond.Wait()
		}
		if p.err != nil {
			p.queue = nil
			p.cond.Broadcast()
			p.mu.Unlock()
			continue
		}
		job := p.queue[0]
		p.queue = p.queue[1:]
		p.pushing = true
		pending := len(p.queue)
		p.cond.Broadcast()
		p.mu.Unlock()

		p.progressf("backup git push\trunning\tsha=%s\tpending=%d", shortSHA(job.sha), pending)
		err := p.pushWithRetry(ctx, job)

		p.mu.Lock()
		p.pushing = false
		if err != nil {
			p.err = err
			p.progressf("backup git push\terror\tsha=%s\terr=%q", shortSHA(job.sha), err.Error())
		} else {
			p.progressf("backup git push\tdone\tsha=%s", shortSHA(job.sha))
		}
		p.cond.Broadcast()
		p.mu.Unlock()
	}
}

func (p *asyncRepoPusher) pushWithRetry(ctx context.Context, job asyncPushJob) error {
	var lastErr error
	for attempt := 1; attempt <= asyncPushMaxAttempts; attempt++ {
		err := pushCommit(ctx, p.cfg, job.sha)
		if err == nil {
			return nil
		}
		lastErr = err
		if hardGitPushRejection(err) || attempt == asyncPushMaxAttempts {
			return err
		}
		delay := time.Duration(attempt*15) * time.Second
		p.progressf("backup git push\tretry\tsha=%s\tattempt=%d\tdelay=%s\terr=%q", shortSHA(job.sha), attempt, delay, err.Error())
		time.Sleep(delay)
	}
	return lastErr
}

func (p *asyncRepoPusher) progressf(format string, args ...any) {
	if p.progress != nil {
		p.progress(format, args...)
	}
}

func hardGitPushRejection(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "GH001") ||
		strings.Contains(text, "Large files detected") ||
		strings.Contains(text, "exceeds GitHub's file size limit") ||
		strings.Contains(text, "pre-receive hook declined")
}

func shortSHA(sha string) string {
	sha = strings.TrimSpace(sha)
	if len(sha) <= 12 {
		return sha
	}
	return sha[:12]
}

func asyncPushError(repo string) error {
	pusher := existingAsyncPusher(repo, nil)
	if pusher == nil {
		return nil
	}
	pusher.mu.Lock()
	defer pusher.mu.Unlock()
	if pusher.err == nil {
		return nil
	}
	return fmt.Errorf("async backup push failed: %w", pusher.err)
}
