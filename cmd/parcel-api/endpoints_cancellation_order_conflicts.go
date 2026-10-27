package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type cancellationOrderConflictsWiring struct {
	conflicts shipmenthttp.CancellationOrderConflictCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[cancellationOrderConflictsWiring]{
		lines: map[string]endpointLine{
			"/cancellation-order-conflicts": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (cancellationOrderConflictsWiring, error) {
			// 读口就是冲突册上「没有对应决定留痕」的行（ADR-0295 决定一）：读面不是编排，不触发决定。
			conflicts, err := pspostgres.NewCancellationOrderConflictCatalogue(sources.db)
			return cancellationOrderConflictsWiring{conflicts: conflicts}, err
		},
		mount: func(channels endpointChannels, wiring cancellationOrderConflictsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 取消先后冲突册查阅（ADR-0295 决定一）：运营查阅，授权边界是租户；与面单交易、渠道择优决定、释放未受理册三口共用委托上下文那一只操作者查阅
				// Intake。解开是「运营决定」能力面上另一个端点，查阅面里没有动作。
				{Pattern: "/cancellation-order-conflicts", Handler: shipmenthttp.NewQueryCancellationOrderConflictsEndpoint(channels.cancellationOrderConflicts(), wiring.conflicts)},
			}
		},
	})
}

func (channels endpointChannels) cancellationOrderConflicts() shipmenthttp.CancellationOrderConflictQueryIntake {
	if channels.operatorRegistries.shipmentOperationsRead != nil {
		return channels.operatorRegistries.shipmentOperationsRead
	}
	return shipmenthttp.UnconfiguredIntake{}
}
