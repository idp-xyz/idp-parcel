package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
)

type settlementRegistrationWiring struct {
	externalFundsFactRegistration           settlementhttp.ExternalFundsFactRegistrar
	externalFundsFactCorrectionRegistration settlementhttp.ExternalFundsFactCorrectionRegistrar
}

func init() {
	registerEndpoints(endpointFeature[settlementRegistrationWiring]{
		lines: map[string]endpointLine{
			"/settlement-external-funds-fact-registrations":            {method: http.MethodPost, face: integrationClientFace(accessidentity.FactExternalFunds)},
			"/settlement-external-funds-fact-correction-registrations": {method: http.MethodPost, face: integrationClientFace(accessidentity.FactExternalFunds)},
		},
		build: func(sources endpointSources) (settlementRegistrationWiring, error) {
			// 外部资金事实采用 / 更正在线登记面编排（票 sa-cc/31，27 裁决 2 第二步）：接真不等操作者
			// Intake——未配置 Intake 拒在编排之前，换真那笔工作在装配点换的只是 Intake；判据同关务登记写面。
			settlementRegistration, err := buildSettlementRegistrationOrchestration(sources.db)
			return settlementRegistrationWiring{
				externalFundsFactRegistration:           settlementRegistration.externalFundsFact,
				externalFundsFactCorrectionRegistration: settlementRegistration.externalFundsFactCorrection,
			}, err
		},
		mount: func(channels endpointChannels, wiring settlementRegistrationWiring) []httpapi.BusinessEndpoint {
			externalFundsFactRegistrationIntake := settlementhttp.ExternalFundsFactRegistrationIntake(channels.integrationClients.settlement)
			externalFundsFactCorrectionIntake := settlementhttp.ExternalFundsFactCorrectionRegistrationIntake(channels.integrationClients.settlement)
			return []httpapi.BusinessEndpoint{
				// 外部资金事实采用 / 更正两口的在线登记口（ADR-0085 决定一，票 sa-cc/31；27 裁决 2 第二步）：
				// 本上下文第一份命令面。两口都走集成客户端族，更正与首登同一份认证与授予（ADR-0151 决定四；票
				// operator-channel/11），隔离放行已撤。路径的事物词取登记 CLI
				// parcel-settlement-register 的命令名（external-funds-fact /
				// external-funds-fact-correction），同一本册在 CLI 与端点两处不换词；前缀随本上下文读口的 /settlement-。
				// 两口在生产上是同一只编排的两个方法，端点表仍各收一参：装配测试才盖得住「采用口接了更正编排」。
				// 外部资金事实进产品只经本上下文的采用（ADR-0137 决定四）：这两口与运营决定能力面上的手工采用口（ADR-0236）是同一只
				// 采用编排的两族入口，CC 那侧没有、也不会有资金事实的在线口。
				{Pattern: "/settlement-external-funds-fact-registrations", Handler: settlementhttp.NewRegisterExternalFundsFactEndpoint(externalFundsFactRegistrationIntake, wiring.externalFundsFactRegistration)},
				{Pattern: "/settlement-external-funds-fact-correction-registrations", Handler: settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(externalFundsFactCorrectionIntake, wiring.externalFundsFactCorrectionRegistration)},
			}
		},
	})
}
