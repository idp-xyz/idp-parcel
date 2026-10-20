package main

import (
	"net/http"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customsCatalogueWiring struct {
	complianceRules    customshttp.RuleCatalogueReader
	caseRegisters      customshttp.CaseRegisterCatalogueReader
	gateConditions     customshttp.GateConditionCatalogueReader
	portsPaths         customshttp.PortsPathsCatalogueReader
	credentials        customshttp.CredentialCatalogueReader
	dutyCollaborations customshttp.DutyCollaborationCatalogueReader
	dutyVerifications  customshttp.DutyVerificationCatalogueReader
}

func init() {
	registerEndpoints(endpointFeature[customsCatalogueWiring]{
		lines: map[string]endpointLine{
			"/customs-compliance-rules":    {method: http.MethodGet, face: operationsReadFace(), probe: "/customs-compliance-rules?registry=case-requirement"},
			"/customs-case-registers":      {method: http.MethodGet, face: operationsReadFace(), probe: "/customs-case-registers?registry=readiness"},
			"/customs-gate-conditions":     {method: http.MethodGet, face: operationsReadFace()},
			"/customs-ports-paths":         {method: http.MethodGet, face: operationsReadFace(), probe: "/customs-ports-paths?registry=candidate-port"},
			"/customs-credentials":         {method: http.MethodGet, face: operationsReadFace()},
			"/customs-duty-collaborations": {method: http.MethodGet, face: operationsReadFace()},
			"/customs-duty-verifications":  {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (customsCatalogueWiring, error) {
			none := customsCatalogueWiring{}
			complianceRules, err := ccpostgres.NewRuleCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			caseRegisters, err := ccpostgres.NewCaseRegisterCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			gateConditions, err := ccpostgres.NewGateConditionCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			portsPaths, err := ccpostgres.NewPortsPathsCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			credentials, err := ccpostgres.NewCredentialCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			// 协作事项与付款核对两册共用一个读适配器（同一迁移的两张表），接两个读口参数
			// ——传两次的是同一个对象，两口各按自己的接口消费（票 sa-cc/10）。
			dutyReconciliation, err := ccpostgres.NewDutyReconciliationCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			return customsCatalogueWiring{
				complianceRules:    complianceRules,
				caseRegisters:      caseRegisters,
				gateConditions:     gateConditions,
				portsPaths:         portsPaths,
				credentials:        credentials,
				dutyCollaborations: dutyReconciliation,
				dutyVerifications:  dutyReconciliation,
			}, nil
		},
		mount: func(channels endpointChannels, wiring customsCatalogueWiring) []httpapi.BusinessEndpoint {
			complianceRulesIntake := channels.complianceRules()
			return []httpapi.BusinessEndpoint{
				{Pattern: "/customs-compliance-rules", Handler: customshttp.NewQueryComplianceRulesEndpoint(complianceRulesIntake, wiring.complianceRules)},
				// 案件配置册、门禁条件册与规则库查阅同属关务租户内运营读面，共用同一个
				// CatalogueQueryIntake 变量，挂操作者查阅读（票 operator-channel/05）。
				// 判据同为那三条（消费所属上下文存储读面、零持久化、作用域为运营侧授权结果）。
				//
				// 门禁条件不并进 /customs-case-registers 的 registry 分派：那三册归 customs-cases
				// 一张页面，门禁两表归 customs-restrictions——分派对应「一页里的页签」，各立入口
				// 对应「各自独立的页」（票 admin-web-page-wiring-frontier/04 的归属裁决、06 实施）。
				{Pattern: "/customs-case-registers", Handler: customshttp.NewQueryCaseRegistersEndpoint(complianceRulesIntake, wiring.caseRegisters)},
				{Pattern: "/customs-gate-conditions", Handler: customshttp.NewQueryGateConditionsEndpoint(complianceRulesIntake, wiring.gateConditions)},
				// 口岸与申报路径两册（票 admin-remainder-mechanism-batch/03）是一张页面的两签
				// 查阅面，共用一个端点按 registry 分派，Intake 与关务运营读面同族同变量。
				{Pattern: "/customs-ports-paths", Handler: customshttp.NewQueryPortsPathsEndpoint(complianceRulesIntake, wiring.portsPaths)},
				// 凭证、税费付款协作事项、税费付款核对三册（票 sa-cc/10）各立入口、不并进任何
				// registry 分派：凭证读签挂 customs-cases 页、协作与核对两签挂 customs-restrictions
				// 页——三册分落两页，且都不是既有分派封闭集里的册（案件配置四册 / 口岸路径两册），
				// 往那些集里加格等于让一个端点的参数集替两页说话。Intake 与关务运营读面同族同变量，
				// 三条判据同上（消费所属上下文存储读面、零持久化、作用域为运营侧授权结果）。
				{Pattern: "/customs-credentials", Handler: customshttp.NewQueryCredentialsEndpoint(complianceRulesIntake, wiring.credentials)},
				{Pattern: "/customs-duty-collaborations", Handler: customshttp.NewQueryDutyCollaborationsEndpoint(complianceRulesIntake, wiring.dutyCollaborations)},
				{Pattern: "/customs-duty-verifications", Handler: customshttp.NewQueryDutyVerificationsEndpoint(complianceRulesIntake, wiring.dutyVerifications)},
			}
		},
	})
}
