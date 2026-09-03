package cmd

import (
	"context"

	adsenseapi "google.golang.org/api/adsense/v2"

	"github.com/openclaw/gogcli/internal/app"
)

var adSenseTestServices = googleServiceTestSupport[adsenseapi.Service, app.AdSenseServiceFactory]{
	newService: adsenseapi.NewService,
	wrap: func(factory func(context.Context, string) (*adsenseapi.Service, error)) app.AdSenseServiceFactory {
		return factory
	},
	services: func(factory app.AdSenseServiceFactory) app.Services {
		return app.Services{AdSense: factory}
	},
}

var (
	newAdSenseTestService                = adSenseTestServices.new
	unexpectedAdSenseTestService         = adSenseTestServices.unexpected
	executeWithAdSenseTestService        = adSenseTestServices.executeWithService
	executeWithAdSenseTestServiceFactory = adSenseTestServices.execute
)
