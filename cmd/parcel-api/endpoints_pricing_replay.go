package main

import (
	"net/http"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type pricingReplayWiring struct {
	evaluationReplay pricinghttp.EvaluationReplayer
}

func init() {
	registerEndpoints(endpointFeature[pricingReplayWiring]{
		lines: map[string]endpointLine{
			"/pricing-evaluation-replays": {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (pricingReplayWiring, error) {
			// 评价回放编排（ADR-0124，票 wiring-baseline-remainder/06）：接真不等操作者 Intake——未配置
			// Intake 拒在编排之前，换真那笔工作在装配点换的只是 Intake。
			replay, err := buildEvaluationReplayOrchestration(sources.db)
			return pricingReplayWiring{evaluationReplay: replay}, err
		},
		mount: func(channels endpointChannels, wiring pricingReplayWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 评价回放的治理触发面（ADR-0124 决定一，票 wiring-baseline-remainder/06）：治理动作也是命令行，
				// 挂操作者渠道的登记册 Intake（同一 ADR 决定六，票 operator-channel/06）——租户来自认证出的身份，任何
				// 采信自报身份或替它填证据层级的 Intake 都不能有；路径按动词叫 `-replays`，判据同复核口叫 `-reviews`。
				// 回放结果不交结算，编排的依赖结构上就没有交付口。
				{Pattern: "/pricing-evaluation-replays", Handler: pricinghttp.NewReplayEvaluationEndpoint(channels.operatorRegistries.pricing, wiring.evaluationReplay)},
			}
		},
	})
}
