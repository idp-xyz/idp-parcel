package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

type trackingProjectionsWiring struct {
	projectionViews visibilityhttp.OperationsProjectionReader
}

func init() {
	registerEndpoints(endpointFeature[trackingProjectionsWiring]{
		lines: map[string]endpointLine{
			"/tracking-projections": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (trackingProjectionsWiring, error) {
			// 运营追踪查阅读的就是投影库本身（ADR-0076）：同一适配器同时是派生编排的
			// ProjectionStore 与查阅端点的读面，不造第二份数据。
			projectionViews, err := vepostgres.NewProjections(sources.db)
			return trackingProjectionsWiring{projectionViews: projectionViews}, err
		},
		mount: func(channels endpointChannels, wiring trackingProjectionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/tracking-projections", Handler: visibilityhttp.NewQueryTrackingProjectionsEndpoint(channels.trackingProjections(), wiring.projectionViews)},
			}
		},
	})
}
