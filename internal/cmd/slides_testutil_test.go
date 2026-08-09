package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/slides/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newSlidesImageDriveTestService(t *testing.T, fileID string, deleted *bool) *drive.Service {
	t.Helper()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/upload/") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": fileID, "webContentLink": "https://drive.google.com/uc?id=" + fileID,
			})
		case strings.Contains(r.URL.Path, "/files/"+fileID+"/permissions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "perm1"})
		case strings.Contains(r.URL.Path, "/files/"+fileID) && r.Method == http.MethodDelete:
			*deleted = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	svc, closeServer := newGoogleTestService(t, handler, drive.NewService)
	t.Cleanup(closeServer)
	return svc
}

func stubSlidesService(svc *slides.Service) app.SlidesServiceFactory {
	return func(context.Context, string) (*slides.Service, error) { return svc, nil }
}

func withSlidesTestService(ctx context.Context, svc *slides.Service) context.Context {
	return withSlidesTestServiceFactory(ctx, stubSlidesService(svc))
}

func withSlidesTestServiceFactory(ctx context.Context, factory app.SlidesServiceFactory) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	runtime := &app.Runtime{}
	if existing, ok := app.FromContext(ctx); ok {
		*runtime = *existing
	}
	runtime.Services.Slides = factory
	return app.WithRuntime(ctx, runtime)
}

func withSlidesAndDriveTestServices(ctx context.Context, slidesSvc *slides.Service, driveSvc *drive.Service) context.Context {
	return withSlidesTestService(withDriveTestService(ctx, driveSvc), slidesSvc)
}
