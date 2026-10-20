package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type shipmentViewsWiring struct {
	requestViews     shipmenthttp.ShipmentRequestViewsReader
	reviewQueue      shipmenthttp.AcceptanceReviewQueueReader
	reviewJudgments  shipmenthttp.RecordedJudgmentsReader
	dispositionQueue shipmenthttp.AuthorizedDispositionQueueReader
}

func init() {
	registerEndpoints(endpointFeature[shipmentViewsWiring]{
		lines: map[string]endpointLine{
			"/shipment-request-views":       {method: http.MethodGet, face: isolatedReadFace()},
			"/acceptance-review-queue":      {method: http.MethodGet, face: isolatedReadFace()},
			"/authorized-disposition-queue": {method: http.MethodGet, face: isolatedReadFace()},
		},
		build: func(sources endpointSources) (shipmentViewsWiring, error) {
			requestViews, err := pspostgres.NewShipmentRequestViews(sources.db)
			if err != nil {
				return shipmentViewsWiring{}, err
			}
			// 复核队列读口就是委托查阅适配器（ports.AcceptanceReviewQueue 在其上补齐，同表同
			// 作用域纪律）；判断读口与形成决定读的是同一批判断行。
			reviewJudgments, err := pspostgres.NewAcceptanceJudgments(sources.db)
			if err != nil {
				return shipmentViewsWiring{}, err
			}
			return shipmentViewsWiring{
				requestViews:    requestViews,
				reviewQueue:     requestViews,
				reviewJudgments: reviewJudgments,
				// 授权处置队列读口就是委托查阅适配器（ports.AuthorizedDispositionQueue 在其上补齐，同表同作用域纪律）。
				dispositionQueue: requestViews,
			}, nil
		},
		mount: func(channels endpointChannels, wiring shipmentViewsWiring) []httpapi.BusinessEndpoint {
			shipmentViewsIntake := channels.shipmentViews()
			return []httpapi.BusinessEndpoint{
				{Pattern: "/shipment-request-views", Handler: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(shipmentViewsIntake, wiring.requestViews)},
				// 复核队列查阅（票 09）：委托查阅面的子集视图，Intake 沿用同一变量——隔离读
				// 准入（ADR-0078）启用时随委托查阅一起换值，不另立第二种准入形。
				{Pattern: "/acceptance-review-queue", Handler: shipmenthttp.NewQueryAcceptanceReviewQueueEndpoint(shipmentViewsIntake, wiring.reviewQueue, wiring.reviewJudgments)},
				// 授权处置队列查阅（票 sa-preacceptance-policy-view/04，ADR-0132 决定二）：同为委托查阅面的子集视图，
				// Intake 沿用同一变量；单份详情复用复核队列的 `?shipmentRequestId=` 分支，这里只有列表。
				{Pattern: "/authorized-disposition-queue", Handler: shipmenthttp.NewQueryAuthorizedDispositionQueueEndpoint(shipmentViewsIntake, wiring.dispositionQueue)},
			}
		},
	})
}
