package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type withdrawalWiring struct {
	// customerWithdrawal 缺席即该行字面量未配置（判 nil，不把带类型的空指针塞进接口）。
	customerWithdrawal *shipmenthttp.CustomerWithdrawalIntake
	withdrawal         shipmenthttp.WithdrawalHandler
}

func init() {
	registerEndpoints(endpointFeature[withdrawalWiring]{
		lines: map[string]endpointLine{
			"/v1/shipment-requests/withdrawals": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityShipmentRequestWithdrawal), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (withdrawalWiring, error) {
			customerWithdrawal, err := buildCustomerWithdrawalIntake(sources.customerClientMinter)
			if err != nil {
				return withdrawalWiring{}, err
			}
			withdrawal, err := buildCustomerWithdrawalOrchestration(sources.db)
			return withdrawalWiring{customerWithdrawal: customerWithdrawal, withdrawal: withdrawal}, err
		},
		mount: func(_ endpointChannels, wiring withdrawalWiring) []httpapi.BusinessEndpoint {
			// 撤回口挂首方客户渠道的 Intake（ADR-0310 取舍 A、B；票 first-party-api/07），理由同 endpoints_submission.go 里
			// submissionIntake 那段注释：单独的变量，两个隔离开关都换不了它。
			intake := shipmenthttp.WithdrawalIntake(shipmenthttp.UnconfiguredIntake{})
			if wiring.customerWithdrawal != nil {
				intake = wiring.customerWithdrawal
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/v1/shipment-requests/withdrawals", Handler: shipmenthttp.NewWithdrawShipmentRequestEndpoint(intake, wiring.withdrawal)},
			}
		},
	})
}
