package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

type transportScheduleWiring struct {
	transportScheduleRegistration tfhttp.TransportScheduleRegistrar
}

func init() {
	registerEndpoints(endpointFeature[transportScheduleWiring]{
		lines: map[string]endpointLine{
			"/transport-fulfillment-transport-schedule-template-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/transport-fulfillment-transport-schedule-registrations":          {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (transportScheduleWiring, error) {
			registration, err := buildTransportScheduleRegistration(sources.db)
			return transportScheduleWiring{transportScheduleRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring transportScheduleWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 周期安排与具体班次登记两口（ADR-0201 决定一）：建班次不搬包裹、不形成生产事实，挂操作者渠道的「登记册配置写」
				// 能力面（ADR-0151 定族判据）；受控 CLI parcel-transport-register 调的是同一个编排。具体班次一口既按（周期安排、
				// 运行日期）物化，也登临时班次。
				{Pattern: "/transport-fulfillment-transport-schedule-template-registrations", Handler: tfhttp.NewRegisterScheduleTemplateEndpoint(channels.operatorRegistries.transport, wiring.transportScheduleRegistration)},
				{Pattern: "/transport-fulfillment-transport-schedule-registrations", Handler: tfhttp.NewRegisterTransportScheduleEndpoint(channels.operatorRegistries.transport, wiring.transportScheduleRegistration)},
			}
		},
	})
}
