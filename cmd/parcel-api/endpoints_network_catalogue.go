package main

import (
	"net/http"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type networkCatalogueWiring struct {
	networkCatalog networkhttp.OperationsCatalogReader
	routePlans     networkhttp.RoutePlanCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[networkCatalogueWiring]{
		lines: map[string]endpointLine{
			"/network-catalog": {method: http.MethodGet, face: operationsReadFace(), probe: "/network-catalog?family=node"},
			"/route-plans":     {method: http.MethodGet, face: operationsReadFace(), probe: "/route-plans?register=initial-route"},
		},
		build: func(sources endpointSources) (networkCatalogueWiring, error) {
			networkCatalog, err := nrpostgres.NewNetworkCatalog(sources.db)
			if err != nil {
				return networkCatalogueWiring{}, err
			}
			routePlans, err := nrpostgres.NewRoutePlanCatalogue(sources.db)
			return networkCatalogueWiring{networkCatalog: networkCatalog, routePlans: routePlans}, err
		},
		mount: func(channels endpointChannels, wiring networkCatalogueWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/network-catalog", Handler: networkhttp.NewQueryNetworkCatalogEndpoint(channels.networkCatalog(), wiring.networkCatalog)},
				{Pattern: "/route-plans", Handler: networkhttp.NewQueryRoutePlansEndpoint(channels.networkCatalog(), wiring.routePlans)},
			}
		},
	})
}
