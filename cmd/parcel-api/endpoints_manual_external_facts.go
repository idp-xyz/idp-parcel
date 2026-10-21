package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

type manualExternalFactsWiring struct {
	externalFundsFactAdoption           settlementhttp.ExternalFundsFactRegistrar
	externalFundsFactCorrectionAdoption settlementhttp.ExternalFundsFactCorrectionRegistrar
	regulatoryCredentialRegistration    customshttp.RegulatoryCredentialRegistrar
}

func init() {
	registerEndpoints(endpointFeature[manualExternalFactsWiring]{
		lines: map[string]endpointLine{
			"/settlement-external-funds-fact-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalFundsFactAdoption),
			},
			"/settlement-external-funds-fact-correction-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionExternalFundsFactAdoption),
			},
			"/customs-regulatory-credential-manual-registrations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionRegulatoryCredentialRegistration),
			},
		},
		build: func(sources endpointSources) (manualExternalFactsWiring, error) {
			// 编排与集成客户端那三口是同一套构造：两族只差 Intake，采用、更正与凭证登记的规则、幂等与答案代数各只有一份。
			settlement, err := buildSettlementRegistrationOrchestration(sources.db)
			if err != nil {
				return manualExternalFactsWiring{}, err
			}
			customs, err := buildCustomsRegistrationOrchestration(sources.db)
			return manualExternalFactsWiring{
				externalFundsFactAdoption:           settlement.externalFundsFact,
				externalFundsFactCorrectionAdoption: settlement.externalFundsFactCorrection,
				regulatoryCredentialRegistration:    customs.regulatoryCredential,
			}, err
		},
		mount: func(channels endpointChannels, wiring manualExternalFactsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 外部资金事实与监管凭证的手工采用口（ADR-0236）：租户的财务、关务人员照银行回单或凭证原件在管理台录入，挂「运营
				// 决定」能力面，两类事实各按自己那一种认证，提交操作者记成操作证据。与集成客户端族那三口并存而不替代：有接口的外部
				// 系统照旧推，没接口的由人照单录，路径带 manual 让两族在端点表上一眼分得开。写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/settlement-external-funds-fact-manual-registrations", Handler: settlementhttp.NewRegisterExternalFundsFactEndpoint(channels.operatorDecisions.settlement, wiring.externalFundsFactAdoption)},
				{Pattern: "/settlement-external-funds-fact-correction-manual-registrations", Handler: settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(channels.operatorDecisions.settlement, wiring.externalFundsFactCorrectionAdoption)},
				{Pattern: "/customs-regulatory-credential-manual-registrations", Handler: customshttp.NewRegisterRegulatoryCredentialEndpoint(channels.operatorDecisions.customs, wiring.regulatoryCredentialRegistration)},
			}
		},
	})
}
