package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customerSupplementWiring struct {
	// customerSupplement 缺席即该行字面量未配置（判 nil，不把带类型的空指针塞进接口）。
	customerSupplement *shipmenthttp.CustomerSupplementIntake
	supplement         shipmenthttp.SupplementHandler
}

func init() {
	registerEndpoints(endpointFeature[customerSupplementWiring]{
		lines: map[string]endpointLine{
			"/v1/shipment-requests/supplements": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityShipmentRequestSupplement), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (customerSupplementWiring, error) {
			customerSupplement, err := buildCustomerSupplementIntake(sources.customerClientMinter)
			if err != nil {
				return customerSupplementWiring{}, err
			}
			// 受控补充编排（ADR-0106 Decision 四）：接进装配后「新提交版本已形成」信封才有生产发布方，停在`等待受控补充`
			// 并已入账的委托才有人续办。
			supplement, err := buildCustomerSupplementOrchestration(sources.db)
			return customerSupplementWiring{customerSupplement: customerSupplement, supplement: supplement}, err
		},
		mount: func(_ endpointChannels, wiring customerSupplementWiring) []httpapi.BusinessEndpoint {
			// 补充口挂首方客户渠道的 Intake（ADR-0310 取舍 B、E；票 first-party-api/08），理由同 endpoints_submission.go 里
			// submissionIntake 那段注释：单独的变量，两个隔离开关都换不了它。
			intake := shipmenthttp.SupplementIntake(shipmenthttp.UnconfiguredIntake{})
			if wiring.customerSupplement != nil {
				intake = wiring.customerSupplement
			}
			return []httpapi.BusinessEndpoint{
				// 受控补充命令口（票 first-tenant-runway/09；ADR-0106 Decision 四）：客户在`已提交`委托上形成同一委托的新提交
				// 版本，编排随新版本落库同事务铸「新提交版本已形成」信封驱动续办。
				{Pattern: "/v1/shipment-requests/supplements", Handler: shipmenthttp.NewFormNewSubmissionVersionEndpoint(intake, wiring.supplement)},
			}
		},
	})
}
