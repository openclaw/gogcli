package cmd

import (
	"context"
	"errors"
	"net/http"
	"testing"

	analyticsadminapi "google.golang.org/api/analyticsadmin/v1beta"
	analyticsdataapi "google.golang.org/api/analyticsdata/v1beta"

	"github.com/openclaw/gogcli/internal/app"
)

func newAnalyticsAdminTestService(t *testing.T, handler http.Handler) *analyticsadminapi.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, analyticsadminapi.NewService)
	t.Cleanup(closeServer)
	return svc
}

func newAnalyticsDataTestService(t *testing.T, handler http.Handler) *analyticsdataapi.Service {
	t.Helper()
	svc, closeServer := newGoogleTestService(t, handler, analyticsdataapi.NewService)
	t.Cleanup(closeServer)
	return svc
}

func fixedAnalyticsAdminTestService(svc *analyticsadminapi.Service) app.AnalyticsAdminServiceFactory {
	return func(context.Context, string) (*analyticsadminapi.Service, error) {
		return svc, nil
	}
}

func fixedAnalyticsDataTestService(svc *analyticsdataapi.Service) app.AnalyticsDataServiceFactory {
	return func(context.Context, string) (*analyticsdataapi.Service, error) {
		return svc, nil
	}
}

func unexpectedAnalyticsDataTestService(t *testing.T, message string) app.AnalyticsDataServiceFactory {
	t.Helper()
	return func(context.Context, string) (*analyticsdataapi.Service, error) {
		t.Fatalf("%s", message)
		return nil, errors.New("unexpected analytics data service call")
	}
}

func executeWithAnalyticsAdminTestService(t *testing.T, args []string, svc *analyticsadminapi.Service) executeTestResult {
	t.Helper()
	return executeWithAnalyticsAdminTestServiceFactory(t, args, fixedAnalyticsAdminTestService(svc))
}

func executeWithAnalyticsAdminTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.AnalyticsAdminServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		AnalyticsAdmin: factory,
	}})
}

func executeWithAnalyticsDataTestService(t *testing.T, args []string, svc *analyticsdataapi.Service) executeTestResult {
	t.Helper()
	return executeWithAnalyticsDataTestServiceFactory(t, args, fixedAnalyticsDataTestService(svc))
}

func executeWithAnalyticsDataTestServiceFactory(
	t *testing.T,
	args []string,
	factory app.AnalyticsDataServiceFactory,
) executeTestResult {
	t.Helper()
	return executeWithTestRuntime(t, args, &app.Runtime{Services: app.Services{
		AnalyticsData: factory,
	}})
}
