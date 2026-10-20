package main

import (
	"net/http"

	governancehttp "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/http"
	govpg "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type governanceRegistersWiring struct {
	governanceRegisters governancehttp.GovernanceRegistryReader
}

func init() {
	registerEndpoints(endpointFeature[governanceRegistersWiring]{
		lines: map[string]endpointLine{
			"/governance-registers": {method: http.MethodGet, face: operationsReadFace(), probe: "/governance-registers?register=authority-interval"},
		},
		build: func(sources endpointSources) (governanceRegistersWiring, error) {
			// 治理登记册读面（票 admin-skeleton-closure-batch/02）：读口无租户参是设计
			// （ADR-0083），适配器读的就是治理登记 CLI 写入的那三张表。
			governanceRegisters, err := govpg.NewGovernanceRegisters(sources.db)
			return governanceRegistersWiring{governanceRegisters: governanceRegisters}, err
		},
		mount: func(channels endpointChannels, wiring governanceRegistersWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 治理登记册三册一口（票 admin-skeleton-closure-batch/02）：治理无租户维是
				// 设计不是缺列（ADR-0083），Intake 因此是本上下文自己的一种准入形：挂操作者查阅读，信封上的租户只用于认证、
				// 不进治理作用域（票 operator-channel/35）。
				{Pattern: "/governance-registers", Handler: governancehttp.NewQueryGovernanceRegistersEndpoint(channels.governanceRegistry(), wiring.governanceRegisters)},
			}
		},
	})
}
