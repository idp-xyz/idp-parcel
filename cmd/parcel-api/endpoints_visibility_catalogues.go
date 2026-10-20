package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

type visibilityCataloguesWiring struct {
	visibilityCatalogues visibilityhttp.VisibilityCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[visibilityCataloguesWiring]{
		lines: map[string]endpointLine{
			"/visibility-catalogues": {method: http.MethodGet, face: operationsReadFace(), probe: "/visibility-catalogues?kind=MILESTONE_MAPPING"},
		},
		build: func(sources endpointSources) (visibilityCataloguesWiring, error) {
			catalogues, err := vepostgres.NewOperationsCatalogue(sources.db)
			return visibilityCataloguesWiring{visibilityCatalogues: catalogues}, err
		},
		mount: func(channels endpointChannels, wiring visibilityCataloguesWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// VE 六类目录查阅与运营追踪查阅同属租户内运营读面，共用同一个
				// OperationsTrackingIntake 变量，同挂操作者查阅读（票 operator-channel/35）。
				{Pattern: "/visibility-catalogues", Handler: visibilityhttp.NewQueryVisibilityCataloguesEndpoint(channels.trackingProjections(), wiring.visibilityCatalogues)},
			}
		},
	})
}
