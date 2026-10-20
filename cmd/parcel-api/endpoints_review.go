package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type reviewWiring struct {
	manualReview shipmenthttp.ManualReviewCompletionHandler
	rejection    shipmenthttp.ActiveRejectionHandler
}

func init() {
	registerEndpoints(endpointFeature[reviewWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/manual-review-completions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionManualReviewCompletion)},
			"/shipment-requests/rejections":                {method: http.MethodPost, face: decisionFace(accessidentity.DecisionActiveRejection)},
		},
		build: func(sources endpointSources) (reviewWiring, error) {
			manualReview, err := buildManualReviewOrchestration(sources.db)
			if err != nil {
				return reviewWiring{}, err
			}
			rejection, err := buildRejectionOrchestration(sources.db)
			return reviewWiring{manualReview: manualReview, rejection: rejection}, err
		},
		mount: func(channels endpointChannels, wiring reviewWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 复核完成与主动拒绝两个命令口（票 09；ADR-0081 的命令面保留条款、ADR-0086）：挂操作者渠道的「运营决定」
				// 能力面（ADR-0151），隔离读准入换不了写行：复核人与决定人取认证出的提交操作者，有没有权由编排问 party-commercial。
				{Pattern: "/shipment-requests/manual-review-completions", Handler: shipmenthttp.NewCompleteManualReviewEndpoint(channels.operatorDecisions.shipment, wiring.manualReview)},
				{Pattern: "/shipment-requests/rejections", Handler: shipmenthttp.NewRejectShipmentRequestEndpoint(channels.operatorDecisions.shipment, wiring.rejection)},
			}
		},
	})
}
