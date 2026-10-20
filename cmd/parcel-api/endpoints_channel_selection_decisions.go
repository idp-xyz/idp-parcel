package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type channelSelectionDecisionsWiring struct {
	channelSelectionDecisions shipmenthttp.ChannelSelectionDecisionsReader
}

func init() {
	registerEndpoints(endpointFeature[channelSelectionDecisionsWiring]{
		lines: map[string]endpointLine{
			"/channel-selection-decisions": {method: http.MethodGet, face: operationsReadFace(), probe: "/channel-selection-decisions?view=tied"},
		},
		build: func(sources endpointSources) (channelSelectionDecisionsWiring, error) {
			// 渠道择优决定查阅读的是决定登记册本尊（票 label-channel/23）：两个契约一只适配器，读面不是编排。
			decisions, err := buildChannelSelectionDecisionRead(sources.db)
			return channelSelectionDecisionsWiring{channelSelectionDecisions: decisions}, err
		},
		mount: func(channels endpointChannels, wiring channelSelectionDecisionsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 渠道择优决定查阅（票 label-channel/23）：并列冲突列表与按标识取一条共用一个端点、按 decisionId 分派。
				// 准入形同面单交易（只有租户维，本册没有账户维可分），自立 Intake 变量、挂操作者查阅读；读口是
				// 决定登记册本尊（两个契约一只适配器）。择优编排今天无生产装配点，接线前这一口读到的是空册，空册是如实答案。
				{Pattern: "/channel-selection-decisions", Handler: shipmenthttp.NewQueryChannelSelectionDecisionsEndpoint(channels.channelSelectionDecisions(), wiring.channelSelectionDecisions)},
			}
		},
	})
}
