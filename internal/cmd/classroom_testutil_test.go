package cmd

import (
	"context"
	"net/http"
	"testing"

	"google.golang.org/api/classroom/v1"

	"github.com/openclaw/gogcli/internal/app"
)

func newClassroomTestService(t *testing.T, handler http.Handler) (*classroom.Service, func()) {
	t.Helper()
	return newGoogleTestService(t, handler, classroom.NewService)
}

func executeWithClassroomTestService(t *testing.T, args []string, svc *classroom.Service) executeTestResult {
	t.Helper()
	return executeWithClassroomTestServiceFactory(t, args, func(context.Context, string) (*classroom.Service, error) {
		return svc, nil
	})
}

func executeWithClassroomTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.ClassroomServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		Classroom: factory,
	}})
}
