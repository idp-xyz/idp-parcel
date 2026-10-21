package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customsRegistrationWiring struct {
	interpretationRuleRegistration      customshttp.InterpretationRuleRegistrar
	gateCatalogRegistration             customshttp.GateCatalogRegistrar
	candidatePortRegistration           customshttp.CandidatePortRegistrar
	declarationPathRegistration         customshttp.DeclarationPathRegistrar
	caseRequirementRegistration         customshttp.CaseRequirementRegistrar
	regulatoryCredentialRegistration    customshttp.RegulatoryCredentialRegistrar
	dutyCollaborationRegistration       customshttp.DutyCollaborationRegistrar
	dutyPaymentVerificationRegistration customshttp.DutyPaymentVerificationRegistrar
}

func init() {
	registerEndpoints(endpointFeature[customsRegistrationWiring]{
		lines: map[string]endpointLine{
			"/customs-interpretation-rule-registrations":       {method: http.MethodPost, face: registryWriteFace()},
			"/customs-gate-catalog-registrations":              {method: http.MethodPost, face: registryWriteFace()},
			"/customs-candidate-port-registrations":            {method: http.MethodPost, face: registryWriteFace()},
			"/customs-declaration-path-registrations":          {method: http.MethodPost, face: registryWriteFace()},
			"/customs-case-requirement-registrations":          {method: http.MethodPost, face: registryWriteFace()},
			"/customs-regulatory-credential-registrations":     {method: http.MethodPost, face: integrationClientFace(accessidentity.FactRegulatoryCredential)},
			"/customs-duty-collaboration-registrations":        {method: http.MethodPost, face: registryWriteFace()},
			"/customs-duty-payment-verification-registrations": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (customsRegistrationWiring, error) {
			customsRegistration, err := buildCustomsRegistrationOrchestration(sources.db)
			return customsRegistrationWiring{
				interpretationRuleRegistration:      customsRegistration.interpretationRule,
				gateCatalogRegistration:             customsRegistration.gateCatalog,
				candidatePortRegistration:           customsRegistration.candidatePort,
				declarationPathRegistration:         customsRegistration.declarationPath,
				caseRequirementRegistration:         customsRegistration.caseRequirement,
				regulatoryCredentialRegistration:    customsRegistration.regulatoryCredential,
				dutyCollaborationRegistration:       customsRegistration.dutyCollaboration,
				dutyPaymentVerificationRegistration: customsRegistration.dutyPaymentVerification,
			}, err
		},
		mount: func(channels endpointChannels, wiring customsRegistrationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 关务四类配置登记写面（ADR-0085，票 admin-write-faces/02 切片 02b）：写准入不
				// 另立形，判据同网络目录登记写面。同一受控 CLI 里改「案上此刻的事实」的那些命令不在本端点族内，
				// 范围判据在票上，此处不复述。
				//
				// 路径不照网络那样带上读口册名：关务的查阅入口本就按事物平铺（/customs-case-registers、
				// /customs-gate-conditions、/customs-ports-paths），四个事物词在整个关务面上唯一，
				// 前缀套前缀只会把路径拉长而分不出更多东西。事物词与读口的 `?registry=` 同源。
				{Pattern: "/customs-interpretation-rule-registrations", Handler: customshttp.NewRegisterInterpretationRuleEndpoint(channels.operatorRegistries.customs, wiring.interpretationRuleRegistration)},
				{Pattern: "/customs-gate-catalog-registrations", Handler: customshttp.NewRegisterGateCatalogEndpoint(channels.operatorRegistries.customs, wiring.gateCatalogRegistration)},
				{Pattern: "/customs-candidate-port-registrations", Handler: customshttp.NewRegisterCandidatePortEndpoint(channels.operatorRegistries.customs, wiring.candidatePortRegistration)},
				{Pattern: "/customs-declaration-path-registrations", Handler: customshttp.NewRegisterDeclarationPathEndpoint(channels.operatorRegistries.customs, wiring.declarationPathRegistration)},
				// 建案要求规则是关务的第五类配置，比另四类晚一步接进来：票 admin-write-faces/02
				// 的关务片把十二个用例分成「配置四类」与「案件事实七类」，四加七只有十一个，漏掉
				// 的第十二个正是它。它有登记用例、有 CLI 命令（case-requirement）、读面早在合规
				// 规则页的册 chip 里，唯独端点表没有它的行——管理台的规则页因此只能把签名写死成
				// 「登记解释规则」，那一半的缺席是页面在替它认账。
				{Pattern: "/customs-case-requirement-registrations", Handler: customshttp.NewRegisterCaseRequirementEndpoint(channels.operatorRegistries.customs, wiring.caseRequirementRegistration)},
				// 凭证、税费付款协作事项、税费付款核对三册的在线登记口（ADR-0085 决定一，票 sa-cc/07
				// 步二；裁决「同族一致」三册都开）：写准入不另立形。凭证口走集成客户端族（ADR-0149；票 operator-channel/11），
				// 隔离放行已撤；租户人员照原件手工登记另走运营决定能力面上的手工口（ADR-0236）。协作与核对两口走操作者渠道的登记写面。
				// 路径的事物词取登记 CLI 的命令名（regulatory-credential / duty-collaboration /
				// duty-payment-verification），同一本册在 CLI 与端点两处不换词；不取读口的册名
				// （/customs-credentials 那组）——读口按册平铺，写口按命令命名，前五个登记口已是这么分的。
				// 协作与核对两口在生产上是同一只编排的两个方法，端点表仍各收一参：装配测试才盖得住
				// 「协作口接了核对编排」。外部资金事实没有在线口：它只经 settlement-accounting 的采用信封
				// 进 CC（ADR-0137 决定四）。
				{Pattern: "/customs-regulatory-credential-registrations", Handler: customshttp.NewRegisterRegulatoryCredentialEndpoint(customshttp.RegulatoryCredentialRegistrationIntake(channels.integrationClients.customs), wiring.regulatoryCredentialRegistration)},
				{Pattern: "/customs-duty-collaboration-registrations", Handler: customshttp.NewRegisterDutyCollaborationEndpoint(channels.operatorRegistries.customs, wiring.dutyCollaborationRegistration)},
				{Pattern: "/customs-duty-payment-verification-registrations", Handler: customshttp.NewRegisterDutyPaymentVerificationEndpoint(channels.operatorRegistries.customs, wiring.dutyPaymentVerificationRegistration)},
			}
		},
	})
}
