package main

import (
	"net/http"

	collectionhttp "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/http"
	crpostgres "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type collectionSubledgersWiring struct {
	codSubledgers collectionhttp.CodSubledgerCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[collectionSubledgersWiring]{
		lines: map[string]endpointLine{
			"/collection-subledgers": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (collectionSubledgersWiring, error) {
			codSubledgers, err := crpostgres.NewCodSubledgerCatalogue(sources.db)
			return collectionSubledgersWiring{codSubledgers: codSubledgers}, err
		},
		mount: func(channels endpointChannels, wiring collectionSubledgersWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 代收分户账册（票 admin-remainder-mechanism-batch/04）：分户账连派生余额与
				// 批次引用一次上列。册只一本，不设分派参数（裁决在端点文件头）；挂操作者查阅读（票 operator-channel/35）。
				{Pattern: "/collection-subledgers", Handler: collectionhttp.NewQueryCodSubledgersEndpoint(channels.collectionCatalogue(), wiring.codSubledgers)},
			}
		},
	})
}
