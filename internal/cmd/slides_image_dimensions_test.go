package cmd

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/openclaw/gogcli/internal/app"
)

func TestPublicImageClientCannotUseProxyResolution(t *testing.T) {
	client := publicImageHTTPClient()
	defer client.CloseIdleConnections()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatal("metadata transport must dial destinations directly so address validation cannot be bypassed through a proxy")
	}
}

func TestPublicImageAddressPolicy(t *testing.T) {
	for _, address := range []string{"8.8.8.8:443", "[2606:4700:4700::1111]:443"} {
		if err := checkPublicImageAddress(address); err != nil {
			t.Errorf("public %s: %v", address, err)
		}
	}
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "192.168.1.1:443", "169.254.169.254:443", "100.100.100.200:443", "0.1.2.3:443", "[::1]:443", "[::ffff:127.0.0.1]:443", "[fd00::1]:443", "[64:ff9b::7f00:1]:443", "[2002:7f00:1::]:443", "example.com:443"} {
		if !errors.Is(checkPublicImageAddress(address), errImageAddressNotPublic) {
			t.Errorf("accepted non-public %s", address)
		}
	}
}

func TestPublicImageRedirectPolicy(t *testing.T) {
	redirect, requestErr := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://cdn.example.com/image", nil)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	redirect.Header.Set("Referer", "https://example.com/image?signature=private-marker")
	if err := checkPublicImageRedirect(redirect, nil); err != nil {
		t.Fatal(err)
	}
	if redirect.Header.Get("Referer") != "" {
		t.Fatal("redirect leaked signed source URL")
	}
	for _, raw := range []string{"http://example.com/image", "https://user:secret@example.com/image"} {
		parsed, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkPublicImageRedirect(&http.Request{URL: parsed}, nil); err == nil {
			t.Errorf("accepted unsafe redirect")
		}
	}
	parsed, err := url.Parse("https://example.com/image")
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPublicImageRedirect(&http.Request{URL: parsed}, make([]*http.Request, 10)); err == nil {
		t.Fatal("accepted redirect loop")
	}
}

func TestFetchImageAspectRatioAnonymousAndBounded(t *testing.T) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image.NewRGBA(image.Rect(0, 0, 4, 2))); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("metadata request carried credentials")
		}
		if r.Header.Get("Range") != "bytes=0-1048575" {
			t.Errorf("unexpected Range: %s", r.Header.Get("Range"))
		}
		_, _ = w.Write(pngData.Bytes())
	}))
	defer srv.Close()
	aspect, err := fetchImageAspectRatio(context.Background(), srv.Client(), srv.URL+"/image.png")
	if err != nil || aspect != 0.5 {
		t.Fatalf("aspect = %v, err = %v", aspect, err)
	}
	if _, err := readPublicImageAspectRatio(context.Background(), srv.URL); !errors.Is(err, errImageAddressNotPublic) {
		t.Fatalf("loopback not blocked: %v", err)
	}
}

type imageMetadataTransport func(*http.Request) (*http.Response, error)

func (f imageMetadataTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImageMetadataDiagnosticsDoNotExposeSignedURL(t *testing.T) {
	client := &http.Client{Transport: imageMetadataTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection failed") })}
	_, err := fetchImageAspectRatio(context.Background(), client, "https://example.com/image?signature=private-marker")
	if err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestImageMetadataReaderLimit(t *testing.T) {
	// A JPEG may carry many APP segments before its dimensions. A header beyond
	// the metadata budget must fail without reading the rest of the response.
	segment := append([]byte{0xff, 0xe1, 0xff, 0xff}, make([]byte, 65533)...)
	data := make([]byte, 2, 2+20*len(segment))
	data[0], data[1] = 0xff, 0xd8
	for range 20 {
		data = append(data, segment...)
	}
	reader := &imageCountingReader{Reader: bytes.NewReader(data)}
	client := &http.Client{Transport: imageMetadataTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader), Header: make(http.Header)}, nil
	})}
	if _, err := fetchImageAspectRatio(context.Background(), client, "https://example.com/image.jpg"); err == nil {
		t.Fatal("accepted oversized metadata")
	}
	if reader.bytes > maxImageHeaderBytes {
		t.Fatalf("read %d bytes", reader.bytes)
	}
}

type imageCountingReader struct {
	io.Reader
	bytes int
}

func (r *imageCountingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytes += n
	return n, err
}

func TestSlidesImageDimensionsRejectInvalidValues(t *testing.T) {
	for _, pair := range [][2]float64{{0, 0}, {-1, 2}, {2, -1}, {math.NaN(), 2}, {2, math.Inf(1)}} {
		if err := validateSlidesImageDimensions(pair[0], pair[1]); err == nil {
			t.Fatalf("accepted %v", pair)
		}
	}
}

func TestRemoteImageAspectRequiresExplicitRuntimeService(t *testing.T) {
	ctx := app.WithRuntime(context.Background(), &app.Runtime{})
	_, _, err := resolveSlidesImageDimensions(ctx, slidesImageSource{imageURL: "https://example.com/image"}, 100, 0)
	if !errors.Is(err, errRuntimeServiceRequired) {
		t.Fatalf("missing runtime service did not fail closed: %v", err)
	}
}
