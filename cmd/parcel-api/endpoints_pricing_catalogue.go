package main

import (
	"net/http"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingCatalogueWiring struct {
	priceCards               pricinghttp.PriceCardCatalogueReader
	referenceSeries          pricinghttp.ReferenceSeriesCatalogueReader
	referenceSeriesCoverage  pricinghttp.ReferenceSeriesCoverageReader
	pendingSeriesEvaluations pricinghttp.PendingSeriesEvaluationReader
	pricingEvaluations       pricinghttp.EvaluationCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[pricingCatalogueWiring]{
		lines: map[string]endpointLine{
			"/pricing-price-cards":                {method: http.MethodGet, face: operationsReadFace()},
			"/pricing-reference-series":           {method: http.MethodGet, face: operationsReadFace()},
			"/pricing-reference-series-coverage":  {method: http.MethodGet, face: operationsReadFace()},
			"/pricing-pending-series-evaluations": {method: http.MethodGet, face: operationsReadFace()},
			"/pricing-evaluations":                {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (pricingCatalogueWiring, error) {
			none := pricingCatalogueWiring{}
			// 主数据目录查阅直接接所属上下文的存储读面（ADR-0077），不绕进应用编排。
			// 同一上下文的多个端点共享同一只读适配器，读的仍是各登记写口背后的那份库。
			pricingCatalog, err := pppostgres.NewOperationsCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			// 覆盖地平线读口（票 pricing-reference-series-operations/05「伴生读口 `ReferenceSeriesCoverageRead`」）：
			// 与目录读面同库，但它要连版本与复核两张表，且在用版本由领域按时刻挑，故自成一只适配器。
			referenceSeriesCoverage, err := pppostgres.NewReferenceSeriesCoverage(sources.db)
			if err != nil {
				return none, err
			}
			// 被挂起评价联动读口（ADR-0105 Decision 五）：只读问题项子表 evaluation_issue，不扫评价快照。
			pendingSeriesEvaluations, err := pppostgres.NewPendingSeriesEvaluations(sources.db)
			if err != nil {
				return none, err
			}
			// 评价册与路由判断两册是业务事实册的查阅面（票 admin-skeleton-closure-batch/03）：
			// 与目录读面同上下文同库，但行形状归各自用例，适配器各自成形。
			pricingEvaluations, err := pppostgres.NewEvaluationCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			return pricingCatalogueWiring{
				priceCards:               pricingCatalog,
				referenceSeries:          pricingCatalog,
				referenceSeriesCoverage:  referenceSeriesCoverage,
				pendingSeriesEvaluations: pendingSeriesEvaluations,
				pricingEvaluations:       pricingEvaluations,
			}, nil
		},
		mount: func(channels endpointChannels, wiring pricingCatalogueWiring) []httpapi.BusinessEndpoint {
			pricingCatalogueIntake := channels.pricingCatalogue()
			return []httpapi.BusinessEndpoint{
				{Pattern: "/pricing-price-cards", Handler: pricinghttp.NewQueryPriceCardsEndpoint(pricingCatalogueIntake, wiring.priceCards)},
				{Pattern: "/pricing-reference-series", Handler: pricinghttp.NewQueryReferenceSeriesEndpoint(pricingCatalogueIntake, wiring.referenceSeries)},
				// 覆盖地平线（票 pricing-reference-series-operations/05「伴生读口 `ReferenceSeriesCoverageRead`」）：一条序列一行，
				// 答的是「还盖得住多久、谁欠一个动作」。时刻源在此处选定——组合根就是挑具体依赖
				// 的地方，端点自己收 CoverageClock 以便传输层测试注入固定时钟。
				{Pattern: "/pricing-reference-series-coverage", Handler: pricinghttp.NewQueryReferenceSeriesCoverageEndpoint(pricingCatalogueIntake, wiring.referenceSeriesCoverage, systemClock{})},
				// 被挂起评价联动（ADR-0105 Decision 五；票 pricing-reference-series-operations/05「被挂起评价联动」）：
				// 因序列未解析而待判断的评价数按序列种类分组，只读问题项子表；与覆盖地平线共用同一时刻源，摘要条并排两格说的才是同一刻。
				{Pattern: "/pricing-pending-series-evaluations", Handler: pricinghttp.NewQueryPendingSeriesEvaluationsEndpoint(pricingCatalogueIntake, wiring.pendingSeriesEvaluations, systemClock{})},
				// 评价册与路由判断两册（票 admin-skeleton-closure-batch/03）：业务事实册的
				// 查阅与目录查阅同属租户内运营读面，各随本上下文既有的 Intake 变量换值，
				// 不为事实册另立第二种准入形（裁决在各端点构造函数注释）。
				{Pattern: "/pricing-evaluations", Handler: pricinghttp.NewQueryEvaluationsEndpoint(pricingCatalogueIntake, wiring.pricingEvaluations)},
			}
		},
	})
}
