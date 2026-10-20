package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type carrierPickupJudgmentWiring struct {
	carrierPickupChain    tfhttp.CarrierPickupChainReader
	carrierPickupJudgment tfhttp.CarrierPickupJudge
}

func init() {
	registerEndpoints(endpointFeature[carrierPickupJudgmentWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-carrier-first-effective-pickups": {
				method: http.MethodGet, face: operationsReadFace(),
				probe: "/transport-fulfillment-carrier-first-effective-pickups?object=SYN-PARCEL-1",
			},
			"/transport-fulfillment-carrier-first-effective-pickup-judgments": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionCarrierFirstEffectivePickupJudgment),
			},
		},
		build: func(sources endpointSources) (carrierPickupJudgmentWiring, error) {
			// 实际承运商首次有效收寄的判断面（票 label-channel/31）：写编排与读口出自同一次装配——读口就是收寄登记册本尊。
			judgment, chain, err := buildCarrierPickupJudgment(sources.db)
			if err != nil {
				return carrierPickupJudgmentWiring{}, err
			}
			return carrierPickupJudgmentWiring{carrierPickupChain: chain, carrierPickupJudgment: judgment}, nil
		},
		mount: func(channels endpointChannels, wiring carrierPickupJudgmentWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 实际承运商首次有效收寄的判断面两口（票 label-channel/31，ADR-0135 决定八）。读口按（租户，载运对象）上列整条
				// 收寄链，是判断人的「这个对象走到哪一版」：零登记零编辑零披露，消费本上下文自己的存储读面，走运输履约查阅同一个
				// Intake 变量。写口是判断方的显式读法，一次判断落的是控制事实——立段、结束取消权、交 parcel-shipment 形成终局：
				// 读写两个隔离开关都换不了它，挂操作者渠道的「运营决定」能力面（ADR-0151 决定一）。
				{Pattern: "/transport-fulfillment-carrier-first-effective-pickups", Handler: tfhttp.NewQueryCarrierFirstEffectivePickupsEndpoint(channels.transportCatalogue(), wiring.carrierPickupChain)},
				{Pattern: "/transport-fulfillment-carrier-first-effective-pickup-judgments", Handler: tfhttp.NewJudgeCarrierFirstEffectivePickupEndpoint(channels.operatorDecisions.transport, wiring.carrierPickupJudgment)},
			}
		},
	})
}
