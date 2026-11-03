package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type customerTrackingViewWiring struct {
	trackingViews visibilityhttp.TrackingViewReader
	// attributions 是读不到视图时问的那一次客户归属（ADR-0246 决定二）。
	attributions veports.ParcelCustomerAccountView
	// customerTrackingView 缺席即字面量未配置（customerTrackingViewQueryIntake 判 nil）。
	customerTrackingView *visibilityhttp.CustomerTrackingViewIntake
	// shipments 是按运单号查询时问 parcel-shipment 的整单读口（ADR-0356 决定四）。
	shipments veports.CustomerShipmentTrackingView
	// customerShipmentTracking 缺席即字面量未配置（customerShipmentTrackingQueryIntake 判 nil）。
	customerShipmentTracking *visibilityhttp.CustomerShipmentTrackingIntake
}

func init() {
	registerEndpoints(endpointFeature[customerTrackingViewWiring]{
		lines: map[string]endpointLine{
			"/v1/customer-tracking-view": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityTrackingViewQuery), ownIntake: true,
			},
			// 按运单号查询（ADR-0356）：同一用例、同一组视图与归属读口，所以同一特性；授予是自己那个词（一口一词）。
			"/v1/customer-shipment-tracking": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityShipmentTrackingQuery), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (customerTrackingViewWiring, error) {
			// 客户追踪视图查阅口的首方 Intake（ADR-0139 决定四；票 demo-chain-gaps/96）：铸造器与提交口同一只，查阅不判准入范围。
			intake, err := buildCustomerTrackingViewIntake(sources.customerClientMinter)
			if err != nil {
				return customerTrackingViewWiring{}, err
			}
			trackingViews, err := vepostgres.NewCustomerViews(sources.db)
			if err != nil {
				return customerTrackingViewWiring{}, err
			}
			attributions, err := buildCustomerTrackingViewAttributions(sources.db)
			if err != nil {
				return customerTrackingViewWiring{}, err
			}
			shipmentIntake, err := buildCustomerShipmentTrackingIntake(sources.customerClientMinter)
			if err != nil {
				return customerTrackingViewWiring{}, err
			}
			shipments, err := buildCustomerShipmentLookup(sources.db)
			return customerTrackingViewWiring{
				trackingViews: trackingViews, attributions: attributions, customerTrackingView: intake,
				shipments: shipments, customerShipmentTracking: shipmentIntake,
			}, err
		},
		mount: func(_ endpointChannels, wiring customerTrackingViewWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/v1/customer-tracking-view", Handler: visibilityhttp.NewQueryCustomerTrackingViewEndpoint(customerTrackingViewQueryIntake(wiring.customerTrackingView), wiring.trackingViews, wiring.attributions)},
				{Pattern: "/v1/customer-shipment-tracking", Handler: visibilityhttp.NewQueryCustomerShipmentTrackingEndpoint(customerShipmentTrackingQueryIntake(wiring.customerShipmentTracking), wiring.shipments, wiring.trackingViews, wiring.attributions)},
			}
		},
	})
}
