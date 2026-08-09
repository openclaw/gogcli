package cmd

import (
	"context"

	searchconsoleapi "google.golang.org/api/searchconsole/v1"

	"github.com/openclaw/gogcli/internal/app"
)

var searchConsoleTestServices = googleServiceTestSupport[searchconsoleapi.Service, app.SearchConsoleServiceFactory]{
	newService: searchconsoleapi.NewService,
	wrap: func(factory func(context.Context, string) (*searchconsoleapi.Service, error)) app.SearchConsoleServiceFactory {
		return factory
	},
	services: func(factory app.SearchConsoleServiceFactory) app.Services {
		return app.Services{SearchConsole: factory}
	},
}

var (
	newSearchConsoleTestService                = searchConsoleTestServices.new
	unexpectedSearchConsoleTestService         = searchConsoleTestServices.unexpected
	executeWithSearchConsoleTestService        = searchConsoleTestServices.executeWithService
	executeWithSearchConsoleTestServiceFactory = searchConsoleTestServices.execute
)
