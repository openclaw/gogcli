package googleapi

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// ErrResponseTooLarge means the expanded provider response exceeded the
// caller's bound. Partial response bytes must not be treated as a result.
var ErrResponseTooLarge = errors.New("provider response exceeds byte limit")

type responseByteLimitKey struct{}

// WithResponseByteLimit bounds expanded response bodies before API decoding.
// A nonpositive value leaves normal clients unchanged.
func WithResponseByteLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, responseByteLimitKey{}, limit)
}

func responseLimitedTransportFromContext(ctx context.Context, base http.RoundTripper) http.RoundTripper {
	limit, _ := ctx.Value(responseByteLimitKey{}).(int64)
	if limit <= 0 {
		return base
	}

	return &responseLimitedTransport{base: base, limit: limit}
}

type responseLimitedTransport struct {
	base  http.RoundTripper
	limit int64
}

//nolint:wrapcheck // RoundTripper preserves transport errors for HTTP callers
func (t *responseLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}

		return nil, err
	}

	if resp.Body == nil {
		return resp, nil
	}
	body := resp.Body
	// net/http transparently decompresses its own gzip requests. Explicit
	// Accept-Encoding also needs expanded-byte accounting.
	if resp.Header.Get("Content-Encoding") == "gzip" && !resp.Uncompressed {
		z, zerr := gzip.NewReader(body)
		if zerr != nil {
			_ = body.Close()
			return nil, fmt.Errorf("decode compressed response: %w", zerr)
		}
		body = &expandedResponseBody{Reader: z, compressed: resp.Body}
		resp.Header.Del("Content-Encoding")
		resp.Header.Del("Content-Length")
		resp.ContentLength = -1
		resp.Uncompressed = true
	}

	if resp.ContentLength > t.limit {
		_ = body.Close()
		return nil, ErrResponseTooLarge
	}
	resp.Body = &responseLimitedBody{body: body, ctx: req.Context(), remaining: t.limit}

	return resp, nil
}

type expandedResponseBody struct {
	*gzip.Reader
	compressed io.Closer
}

func (b *expandedResponseBody) Close() error {
	return errors.Join(b.Reader.Close(), b.compressed.Close())
}

type responseLimitedBody struct {
	body      io.ReadCloser
	ctx       context.Context //nolint:containedctx // body reads must follow the owning HTTP request cancellation
	remaining int64
	once      sync.Once
	closeErr  error
	terminal  error
}

//nolint:wrapcheck // io.Reader requires exact EOF and context cancellation semantics
func (b *responseLimitedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	if b.terminal != nil {
		return 0, b.terminal
	}

	if err := b.ctx.Err(); err != nil {
		b.terminal = err
		_ = b.Close()

		return 0, err
	}

	if b.remaining == 0 {
		var probe [1]byte

		n, err := b.body.Read(probe[:])
		if n > 0 {
			b.terminal = ErrResponseTooLarge
			_ = b.Close()

			return 0, b.terminal
		}

		if err != nil {
			b.terminal = err
			_ = b.Close()
		}

		return 0, err
	}

	if int64(len(p)) > b.remaining {
		p = p[:int(b.remaining)]
	}
	n, err := b.body.Read(p)

	b.remaining -= int64(n)
	if err != nil {
		b.terminal = err
		_ = b.Close()
	}

	return n, err
}

func (b *responseLimitedBody) Close() error {
	b.once.Do(func() { b.closeErr = b.body.Close() })
	return b.closeErr
}
