package main

import (
	"net/http"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

type operationsRecordsWiring struct {
	nodeOperationsRecords       nodeopshttp.ReviewCatalogueReader
	transportFulfillmentRecords tfhttp.ReviewCatalogueReader
	handoverScopeSummary        tfhttp.HandoverScopeSummarizer
}

func init() {
	registerEndpoints(endpointFeature[operationsRecordsWiring]{
		lines: map[string]endpointLine{
			"/node-operations-records":                      {method: http.MethodGet, face: operationsReadFace(), probe: "/node-operations-records?registry=reception"},
			"/transport-fulfillment-records":                {method: http.MethodGet, face: operationsReadFace(), probe: "/transport-fulfillment-records?registry=transport-schedule"},
			"/transport-fulfillment-handover-scope-summary": {method: http.MethodGet, face: operationsReadFace(), probe: "/transport-fulfillment-handover-scope-summary?scope=scope-1"},
		},
		build: func(sources endpointSources) (operationsRecordsWiring, error) {
			// 节点作业与运输履约查阅页读面（票 admin-skeleton-closure-batch/05）：只是查阅
			// 读口，与 buildReceptionOrchestration/buildDeliveryOrchestration 构造的命令
			// 编排互不相知——读面不是编排，查阅不触发判断、派生或披露。
			nodeOperationsRecords, err := nopostgres.NewReviewCatalogue(sources.db)
			if err != nil {
				return operationsRecordsWiring{}, err
			}
			transportFulfillmentRecords, err := tfpostgres.NewReviewCatalogue(sources.db)
			if err != nil {
				return operationsRecordsWiring{}, err
			}
			// 交接范围汇总读用例（票 admin-web-audit-followups/06）。读口取交接登记册本尊：它同时
			// 实现写侧的幂等存取与 ports.HandoverScopeView，两个契约一只适配器（分口的理由在
			// ports.HandoverScopeView 注释）。这一格与上面的查阅读口不同——交入装配点的是应用用例不是
			// 读口，因为计数只能由领域派生；在这里预先数一遍就等于为同一形状立第二个口径。
			transportHandovers, err := tfpostgres.NewTransportHandovers(sources.db)
			if err != nil {
				return operationsRecordsWiring{}, err
			}
			return operationsRecordsWiring{
				nodeOperationsRecords:       nodeOperationsRecords,
				transportFulfillmentRecords: transportFulfillmentRecords,
				handoverScopeSummary:        tfapp.NewSummarizeHandoverScopeHandler(transportHandovers),
			}, nil
		},
		mount: func(channels endpointChannels, wiring operationsRecordsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 节点作业与运输履约查阅页（票 admin-skeleton-closure-batch/05）各一口按 registry 分派：分派对应「一页里的页签」。
				// 命令行挂各自上下文的作业事实渠道；查阅行挂本上下文的操作者查阅读（票 operator-channel/35），两个隔离开关都换不了
				// 这几行。
				{Pattern: "/node-operations-records", Handler: nodeopshttp.NewQueryNodeOperationsRecordsEndpoint(channels.nodeOperationsCatalogue(), wiring.nodeOperationsRecords)},
				{Pattern: "/transport-fulfillment-records", Handler: tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(channels.transportCatalogue(), wiring.transportFulfillmentRecords)},
				// 交接范围汇总（票 admin-web-audit-followups/06，读面来自 tf-unwired-seven/03）。
				// 它是本装配表上第一行第二参不是读口而是**应用读用例**的查阅端点：汇总是派生量，
				// 只能由 domain.SummarizeHandovers 派生（理由在 tfhttp.HandoverScopeSummarizer）。
				// 读用例零登记零编辑零披露，查阅的性质没变，因此 Intake 随运输履约查阅同一个变量，同挂操作者查阅读。
				{Pattern: "/transport-fulfillment-handover-scope-summary", Handler: tfhttp.NewQueryHandoverScopeSummaryEndpoint(channels.transportCatalogue(), wiring.handoverScopeSummary)},
			}
		},
	})
}
