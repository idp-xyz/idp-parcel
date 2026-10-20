package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

type customerTrackingViewWiring struct {
	trackingViews visibilityhttp.TrackingViewReader
	// customerTrackingView 缺席即字面量未配置（customerTrackingViewQueryIntake 判 nil）。
	customerTrackingView *visibilityhttp.CustomerTrackingViewIntake
}

func init() {
	registerEndpoints(endpointFeature[customerTrackingViewWiring]{
		lines: map[string]endpointLine{
			"/customer-tracking-view": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityTrackingViewQuery), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (customerTrackingViewWiring, error) {
			// 客户追踪视图查阅口的首方 Intake（ADR-0139 决定四；票 demo-chain-gaps/96）：铸造器与提交口同一只，查阅不判准入范围。
			intake, err := buildCustomerTrackingViewIntake(sources.customerClientMinter)
			if err != nil {
				return customerTrackingViewWiring{}, err
			}
			trackingViews, err := vepostgres.NewCustomerViews(sources.db)
			return customerTrackingViewWiring{trackingViews: trackingViews, customerTrackingView: intake}, err
		},
		mount: func(_ endpointChannels, wiring customerTrackingViewWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/customer-tracking-view", Handler: visibilityhttp.NewQueryCustomerTrackingViewEndpoint(customerTrackingViewQueryIntake(wiring.customerTrackingView), wiring.trackingViews)},
			}
		},
	})
}
