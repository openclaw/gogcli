package cmd

import (
	"context"

	"github.com/steipete/goplaces"
)

type placesClient interface {
	Resolve(ctx context.Context, req goplaces.LocationResolveRequest) (goplaces.LocationResolveResponse, error)
	DetailsWithOptions(ctx context.Context, req goplaces.DetailsRequest) (goplaces.PlaceDetails, error)
}

var newPlacesClient = func(opts goplaces.Options) placesClient {
	return goplaces.NewClient(opts)
}
