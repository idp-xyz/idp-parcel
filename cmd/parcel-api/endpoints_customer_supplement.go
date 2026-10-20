package main

import (
	"net/http"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type customerSupplementWiring struct {
	supplement shipmenthttp.SupplementHandler
}

func init() {
	registerEndpoints(endpointFeature[customerSupplementWiring]{
		lines: map[string]endpointLine{
			"/shipment-requests/supplements": {method: http.MethodPost, face: unconfiguredFace()},
		},
		build: func(sources endpointSources) (customerSupplementWiring, error) {
			// 受控补充编排（ADR-0106 Decision 四）：此前它只活在测试里，接进装配后「新提交版本已形成」
			// 信封才有生产发布方，停在`等待受控补充`并已入账的委托才有人续办。
			supplement, err := buildCustomerSupplementOrchestration(sources.db)
			return customerSupplementWiring{supplement: supplement}, err
		},
		mount: func(_ endpointChannels, wiring customerSupplementWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 受控补充命令口（票 first-tenant-runway/09；ADR-0106 Decision 四）：客户在`已提交`委托上形成
				// 同一委托的新提交版本，编排随新版本落库同事务铸「新提交版本已形成」信封驱动续办。它是客户
				// 渠道的写行，同挂字面量 UnconfiguredIntake{}；谁能替哪个客户账户补充、基准版本怎么译，属
				// 客户渠道（`PAR-INT-01`；首方渠道见 ADR-0139 草案，接受与否归用户），隔离读准入与隔离
				// 提交放行都换不了这一行。
				{Pattern: "/shipment-requests/supplements", Handler: shipmenthttp.NewFormNewSubmissionVersionEndpoint(shipmenthttp.UnconfiguredIntake{}, wiring.supplement)},
			}
		},
	})
}
