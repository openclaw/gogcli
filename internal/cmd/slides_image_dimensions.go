package cmd

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // register GIF metadata decoder
	_ "image/jpeg" // register JPEG metadata decoder
	_ "image/png"  // register PNG metadata decoder
	"io"
	"math"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"syscall"
	"time"
)

const maxImageHeaderBytes = 1 << 20

var errImageAddressNotPublic = errors.New("image metadata requires a public network address")

var imageMetadataBlockedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

var imageMetadataIPv6Global = netip.MustParsePrefix("2000::/3")

func validateSlidesImageDimensions(width, height float64) error {
	if math.IsNaN(width) || math.IsInf(width, 0) || math.IsNaN(height) || math.IsInf(height, 0) {
		return usage("--width and --height must be finite")
	}
	if width < 0 || height < 0 {
		return usage("--width and --height cannot be negative")
	}
	if width == 0 && height == 0 {
		return usage("provide --width or --height greater than 0")
	}
	return nil
}

func resolveSlidesImageDimensions(ctx context.Context, source slidesImageSource, width, height float64) (float64, float64, error) {
	if width > 0 && height > 0 {
		return width, height, nil
	}
	var aspect float64
	var err error
	if source.imageURL == "" {
		aspect, err = imageAspectRatio(source.localPath)
	} else {
		aspect, err = publicImageAspect(ctx, source.imageURL)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("determine image aspect ratio (provide both --width and --height to skip): %w", err)
	}
	if aspect <= 0 || math.IsNaN(aspect) || math.IsInf(aspect, 0) {
		return 0, 0, fmt.Errorf("invalid image aspect ratio")
	}
	if width == 0 {
		width = height / aspect
	} else {
		height = width * aspect
	}
	if err := validateSlidesImageDimensions(width, height); err != nil {
		return 0, 0, err
	}
	if width == 0 || height == 0 {
		return 0, 0, usage("resolved image dimensions must be greater than 0")
	}
	return width, height, nil
}

func imageAspectRatio(path string) (float64, error) {
	file, err := os.Open(path) //nolint:gosec // explicit local image input
	if err != nil {
		return 0, fmt.Errorf("open image: %w", err)
	}
	defer file.Close()
	return decodeImageAspectRatio(file)
}

func decodeImageAspectRatio(reader io.Reader) (float64, error) {
	config, _, err := image.DecodeConfig(reader)
	if err != nil {
		return 0, fmt.Errorf("decode image config: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return 0, fmt.Errorf("image dimensions must be greater than 0")
	}
	return float64(config.Height) / float64(config.Width), nil
}

func publicImageHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		ControlContext: func(_ context.Context, _, address string, _ syscall.RawConn) error {
			return checkPublicImageAddress(address)
		},
	}
	// Validate every resolved peer, including redirects. Proxies are deliberately
	// disabled: a CONNECT proxy would resolve the destination outside this guard.
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 10 * time.Second,
		ForceAttemptHTTP2: true,
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: checkPublicImageRedirect}
}

func readPublicImageAspectRatio(ctx context.Context, imageURL string) (float64, error) {
	client := publicImageHTTPClient()
	defer client.CloseIdleConnections()
	return fetchImageAspectRatio(ctx, client, imageURL)
}

func checkPublicImageRedirect(request *http.Request, via []*http.Request) error {
	request.Header.Del("Referer") // A signed source URL must not reach the redirect host.
	if len(via) >= 10 {
		return errors.New("too many image redirects")
	}
	_, err := resolveSlidesImageSource("", request.URL.String())
	return err
}

func checkPublicImageAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errImageAddressNotPublic
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return errImageAddressNotPublic
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return errImageAddressNotPublic
	}
	if ip.Is6() && !imageMetadataIPv6Global.Contains(ip) {
		return errImageAddressNotPublic
	}
	for _, prefix := range imageMetadataBlockedNetworks {
		if prefix.Contains(ip) {
			return errImageAddressNotPublic
		}
	}
	return nil
}

func fetchImageAspectRatio(ctx context.Context, client *http.Client, imageURL string) (float64, error) {
	if _, err := resolveSlidesImageSource("", imageURL); err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return 0, fmt.Errorf("create image metadata request")
	}
	request.Header.Set("Range", fmt.Sprintf("bytes=0-%d", maxImageHeaderBytes-1))
	response, err := client.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err // Keep signed query parameters out of diagnostics.
		}
		return 0, fmt.Errorf("fetch image metadata: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("image metadata request returned HTTP %d", response.StatusCode)
	}
	reader := &io.LimitedReader{R: response.Body, N: maxImageHeaderBytes}
	aspect, err := decodeImageAspectRatio(reader)
	if err != nil && reader.N == 0 {
		return 0, fmt.Errorf("image dimensions not found within the %d-byte header budget", maxImageHeaderBytes)
	}
	return aspect, err
}
