package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type segmentCommissionAdoptionWiring struct {
	segmentCommissionAdoption       tfhttp.SegmentCommissionAdopter
	transportCommissionCancellation tfhttp.TransportCommissionCanceller
	bookingRequestCancellation      tfhttp.BookingRequestCanceller
}

func init() {
	registerEndpoints(endpointFeature[segmentCommissionAdoptionWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-segment-commission-adoptions": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionFulfillmentSegmentCommissionAdoption),
			},
			"/transport-fulfillment-transport-commission-cancellations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionTransportCommissionCancellation),
			},
			"/transport-fulfillment-booking-request-cancellations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionBookingRequestCancellation),
			},
		},
		build: func(sources endpointSources) (segmentCommissionAdoptionWiring, error) {
			// 履约段采用运输委托口（ADR-0204）。
			adoption, err := buildSegmentCommissionAdoption(sources.db)
			if err != nil {
				return segmentCommissionAdoptionWiring{}, err
			}
			// 取消委托与取消订舱两口（ADR-0204 决定三）。
			cancellationFaces, err := buildCancellationFaces(sources.db)
			return segmentCommissionAdoptionWiring{
				segmentCommissionAdoption:       adoption,
				transportCommissionCancellation: cancellationFaces,
				bookingRequestCancellation:      cancellationFaces,
			}, err
		},
		mount: func(channels endpointChannels, wiring segmentCommissionAdoptionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 履约段采用运输委托口（ADR-0204）：声明一个在册的实际履约段在一份已提交的委托下履约，委托随之标为已开始，是操作者做出的
				// 生产决定，挂「运营决定」能力面的履约段采用运输委托一种。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-segment-commission-adoptions", Handler: tfhttp.NewAdoptSegmentCommissionEndpoint(channels.operatorDecisions.transport, wiring.segmentCommissionAdoption)},
				// 取消委托口与取消订舱口（ADR-0204 决定三；ADR-0202 决定一）：只停尚未开始的范围，已开始答运输已开始。各按自己那一种运营
				// 决定认证——能提交的不因此能取消。写开关换不了它们（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-transport-commission-cancellations", Handler: tfhttp.NewCancelTransportCommissionEndpoint(channels.operatorDecisions.transport, wiring.transportCommissionCancellation)},
				{Pattern: "/transport-fulfillment-booking-request-cancellations", Handler: tfhttp.NewCancelBookingRequestEndpoint(channels.operatorDecisions.transport, wiring.bookingRequestCancellation)},
			}
		},
	})
}
