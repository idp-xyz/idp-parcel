package main

import (
	"net/http"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type deliveryScopeCatalogueWiring struct {
	deliveryScopeRecords nodeopshttp.DeliveryScopeCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[deliveryScopeCatalogueWiring]{
		lines: map[string]endpointLine{
			"/node-operations-delivery-pre-advices": {method: http.MethodGet, face: operationsReadFace()},
			"/node-operations-delivery-pre-advices/{preAdviceId}": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/node-operations-delivery-pre-advices/SYN-PREADV-01",
			},
			"/node-operations-delivery-handover-notes": {method: http.MethodGet, face: operationsReadFace()},
			"/node-operations-delivery-handover-notes/{noteId}": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/node-operations-delivery-handover-notes/SYN-HN-01",
			},
		},
		build: func(sources endpointSources) (deliveryScopeCatalogueWiring, error) {
			// 送站预报与送站交接单的运营查阅读口（票 ps-external-mark-relations/13），同节点作业查阅页读面是读口，不是编排。
			records, err := nopostgres.NewDeliveryScopeCatalogue(sources.db)
			return deliveryScopeCatalogueWiring{deliveryScopeRecords: records}, err
		},
		mount: func(channels endpointChannels, wiring deliveryScopeCatalogueWiring) []httpapi.BusinessEndpoint {
			nodeOperationsCatalogueIntake := channels.nodeOperationsCatalogue()
			return []httpapi.BusinessEndpoint{
				// 送站预报与送站交接单的运营查阅读（UC-NO-006「权限、隔离与审计」；票 ps-external-mark-relations/13）：与节点作业查阅页同挂主数据与
				// 运营查阅读那一个 Intake，列表一口、按标识读一口。路径随查阅面的连字符前缀，不挂在命令口的 /node-operations/ 下：同一路径上的
				// GET 会与开立口、提交口撞成一个处理器。
				{Pattern: "/node-operations-delivery-pre-advices", Handler: nodeopshttp.NewQueryDeliveryPreAdvicesEndpoint(nodeOperationsCatalogueIntake, wiring.deliveryScopeRecords)},
				{Pattern: "/node-operations-delivery-pre-advices/{preAdviceId}", Handler: nodeopshttp.NewQueryDeliveryPreAdviceEndpoint(nodeOperationsCatalogueIntake, wiring.deliveryScopeRecords)},
				{Pattern: "/node-operations-delivery-handover-notes", Handler: nodeopshttp.NewQueryDeliveryHandoverNotesEndpoint(nodeOperationsCatalogueIntake, wiring.deliveryScopeRecords)},
				{Pattern: "/node-operations-delivery-handover-notes/{noteId}", Handler: nodeopshttp.NewQueryDeliveryHandoverNoteEndpoint(nodeOperationsCatalogueIntake, wiring.deliveryScopeRecords)},
			}
		},
	})
}
