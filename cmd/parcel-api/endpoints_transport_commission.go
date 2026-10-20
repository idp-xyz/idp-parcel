package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type transportCommissionWiring struct {
	transportCommissionSubmission tfhttp.TransportCommissionSubmitter
	bookingRequestSubmission      tfhttp.BookingRequestSubmitter
	bookingAnswer                 tfhttp.BookingAnswerer
}

func init() {
	registerEndpoints(endpointFeature[transportCommissionWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-transport-commission-submissions": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionTransportCommissionSubmission)},
			"/transport-fulfillment-booking-request-submissions":      {method: http.MethodPost, face: decisionFace(accessidentity.DecisionBookingRequestSubmission)},
			"/transport-fulfillment/booking-answers":                  {method: http.MethodPost, face: integrationClientFace(accessidentity.FactCarrierBookingAnswer)},
		},
		build: func(sources endpointSources) (transportCommissionWiring, error) {
			submission, err := buildTransportCommissionSubmission(sources.db)
			if err != nil {
				return transportCommissionWiring{}, err
			}
			// 订舱提交与承运答复两口（ADR-0202 决定一、二、五）。
			bookingFaces, err := buildBookingFaces(sources.db)
			return transportCommissionWiring{
				transportCommissionSubmission: submission,
				bookingRequestSubmission:      bookingFaces,
				bookingAnswer:                 bookingFaces,
			}, err
		},
		mount: func(channels endpointChannels, wiring transportCommissionWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 运输委托提交口（ADR-0202 决定一、三）：运营人员向外部服务提供方提出运输范围与条件，是管理台上作出的生产决定，挂操作者
				// 渠道的「运营决定」能力面、按自己的决定种类认证（ADR-0151 定族判据）；协议在提交时点经 party-commercial 核过，快照与
				// 采购责任法人取自核对结果。
				{Pattern: "/transport-fulfillment-transport-commission-submissions", Handler: tfhttp.NewSubmitTransportCommissionEndpoint(channels.operatorDecisions.transport, wiring.transportCommissionSubmission)},
				// 订舱提交口（ADR-0202 决定一、五）：在一份已提交的委托下就一个在册具体班次申请舱位，是管理台上作出的生产决定，挂
				// 「运营决定」能力面的订舱提交一种。写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment-booking-request-submissions", Handler: tfhttp.NewSubmitBookingRequestEndpoint(channels.operatorDecisions.transport, wiring.bookingRequestSubmission)},
				// 承运订舱答复口（ADR-0202 决定二、五）：提交方是承运系统，挂集成客户端族的承运订舱答复一类；答复方的来源身份取自客户端
				// 绑定、须等于订舱所属委托的服务提供方。发行方参数未设时答未配置；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/transport-fulfillment/booking-answers", Handler: tfhttp.NewAnswerBookingEndpoint(tfhttp.BookingAnswerIntake(channels.integrationClients.transport), wiring.bookingAnswer)},
			}
		},
	})
}
