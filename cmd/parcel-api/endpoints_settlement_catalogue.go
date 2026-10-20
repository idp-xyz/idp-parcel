package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
)

type settlementCatalogueWiring struct {
	settlementCharges           settlementhttp.ChargeCatalogueReader
	settlementStatements        settlementhttp.StatementCatalogueReader
	settlementFundsApplications settlementhttp.FundsApplicationCatalogueReader
	settlementOperatingResults  settlementhttp.OperatingCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[settlementCatalogueWiring]{
		lines: map[string]endpointLine{
			"/settlement-charges":            {method: http.MethodGet, face: operationsReadFace(), probe: "/settlement-charges?registry=customer-charge"},
			"/settlement-statements":         {method: http.MethodGet, face: operationsReadFace(), probe: "/settlement-statements?registry=customer-statement"},
			"/settlement-funds-applications": {method: http.MethodGet, face: operationsReadFace()},
			"/settlement-operating-results":  {method: http.MethodGet, face: operationsReadFace(), probe: "/settlement-operating-results?registry=operating-result"},
		},
		build: func(sources endpointSources) (settlementCatalogueWiring, error) {
			none := settlementCatalogueWiring{}
			// 结算与核算四页各接自己的读适配器：四本目录读的是同一份库，但行形状与所属用例
			// 各不相同，合成一个适配器就得把四组读口挤进一个类型（判据同各读口的分册裁决）。
			settlementCharges, err := sapostgres.NewChargeCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			settlementStatements, err := sapostgres.NewStatementCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			settlementFundsApplications, err := sapostgres.NewFundsApplicationCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			settlementOperatingResults, err := sapostgres.NewOperatingCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			return settlementCatalogueWiring{
				settlementCharges:           settlementCharges,
				settlementStatements:        settlementStatements,
				settlementFundsApplications: settlementFundsApplications,
				settlementOperatingResults:  settlementOperatingResults,
			}, nil
		},
		mount: func(channels endpointChannels, wiring settlementCatalogueWiring) []httpapi.BusinessEndpoint {
			settlementCatalogueIntake := channels.settlementCatalogue()
			return []httpapi.BusinessEndpoint{
				// 结算与核算四页（票 admin-skeleton-closure-batch/04）各立入口，四行共用本上下文
				// 自己的 Intake 变量：作用域形状同为租户一维，责任法人、结算账户与币种是账上的
				// 归属维不是查阅方身份，分设只会让装配点看起来能只配一半。哪几本册进哪个入口、
				// 资金冻结与运营结算余额为何不在其中，裁决在各构造函数的注释，此处不复述。
				{Pattern: "/settlement-charges", Handler: settlementhttp.NewQuerySettlementChargesEndpoint(settlementCatalogueIntake, wiring.settlementCharges)},
				{Pattern: "/settlement-statements", Handler: settlementhttp.NewQuerySettlementStatementsEndpoint(settlementCatalogueIntake, wiring.settlementStatements)},
				{Pattern: "/settlement-funds-applications", Handler: settlementhttp.NewQuerySettlementFundsApplicationsEndpoint(settlementCatalogueIntake, wiring.settlementFundsApplications)},
				{Pattern: "/settlement-operating-results", Handler: settlementhttp.NewQuerySettlementOperatingResultsEndpoint(settlementCatalogueIntake, wiring.settlementOperatingResults)},
			}
		},
	})
}
