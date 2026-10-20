package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

type effectiveTimeJudgmentWiring struct {
	externalTrackingFactReview tfhttp.ExternalTrackingFactReviewReader
	effectiveTimeJudgment      tfhttp.EffectiveTimeJudge
}

func init() {
	registerEndpoints(endpointFeature[effectiveTimeJudgmentWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-external-tracking-facts": {
				method: http.MethodGet, face: operationsReadFace(),
				probe: "/transport-fulfillment-external-tracking-facts?source=SYN-SOURCE-1&view=pending",
			},
			"/transport-fulfillment-effective-time-judgments": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionEffectiveTimeJudgment)},
		},
		build: func(sources endpointSources) (effectiveTimeJudgmentWiring, error) {
			// 外部承运轨迹事实的判断面（票 label-channel/21）：读口取事实登记册本尊——它同时实现写侧的幂等存取与
			// ports.ExternalTrackingFactReviewRead，两个契约一只适配器（判据同交接范围汇总那格）；写编排另建，事务边界
			// 归装配点。
			review, err := tfpostgres.NewExternalTrackingFacts(sources.db)
			if err != nil {
				return effectiveTimeJudgmentWiring{}, err
			}
			judgment, err := buildEffectiveTimeJudgment(sources.db)
			return effectiveTimeJudgmentWiring{externalTrackingFactReview: review, effectiveTimeJudgment: judgment}, err
		},
		mount: func(channels endpointChannels, wiring effectiveTimeJudgmentWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 外部承运轨迹事实的有效时间判断面两口（ADR-0085，票 label-channel/21）。读口按（租户，轨迹源）上列当前版
				// （待判断 / 全部），是判断人的「该判哪几条」：零登记零编辑零披露，消费本上下文自己的存储读面，走运输履约
				// 查阅同一个 Intake 变量（操作者查阅读）。写口是所有者的显式判断（ADR-0102 决定三第一种来源），一次判断就把事实交给
				// visibility-exception 进客户可见面：读写两个隔离开关都换不了它，挂操作者渠道的「运营决定」能力面（ADR-0151 决定一）。路径取读面册名前缀 `transport-fulfillment-`——两口都是运营侧
				// 动作，不是承运方回传口。
				{Pattern: "/transport-fulfillment-external-tracking-facts", Handler: tfhttp.NewQueryExternalTrackingFactsEndpoint(channels.transportCatalogue(), wiring.externalTrackingFactReview)},
				{Pattern: "/transport-fulfillment-effective-time-judgments", Handler: tfhttp.NewJudgeEffectiveTimeEndpoint(channels.operatorDecisions.transport, wiring.effectiveTimeJudgment)},
			}
		},
	})
}
