package main

import (
	"net/http"

	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type networkRegistrationWiring struct {
	networkCatalogRegistration networkhttp.CatalogRegistrar
}

func init() {
	registerEndpoints(endpointFeature[networkRegistrationWiring]{
		lines: map[string]endpointLine{
			"/network-catalog-node-registrations":                    {method: http.MethodPost, face: registryWriteFace()},
			"/network-catalog-connection-registrations":              {method: http.MethodPost, face: registryWriteFace()},
			"/network-catalog-line-registrations":                    {method: http.MethodPost, face: registryWriteFace()},
			"/network-catalog-service-area-registrations":            {method: http.MethodPost, face: registryWriteFace()},
			"/network-catalog-service-calendar-registrations":        {method: http.MethodPost, face: registryWriteFace()},
			"/network-catalog-availability-adjustment-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/network-catalog-route-strategy-registrations":          {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (networkRegistrationWiring, error) {
			// 网络七族、关务四类与 VE 六类登记写面编排（票 admin-write-faces/02 切片
			// 02a/02b/02d）：判据同价卡首切片，接真不等墙降。
			registration, err := buildNetworkCatalogRegistrationOrchestration(sources.db)
			return networkRegistrationWiring{networkCatalogRegistration: registration}, err
		},
		mount: func(channels endpointChannels, wiring networkRegistrationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 网络目录七族登记写面（ADR-0085，票 admin-write-faces/02 切片 02a）：登记是命令
				// 行，挂操作者渠道的登记册 Intake（ADR-0100 决定四，票 operator-channel/04）——写准入不另立形，查阅行的 Intake
				// 变量换不了写行。
				//
				// 路径取「读口册名 + 该族原词 + -registrations」：族词与查阅的 `?family=`、登记
				// CLI 的 `-kind` 逐字同一个，同一本册在三处不换词。一族一个端点而不用 `?family=`
				// 把七族塑进一个口：七族的命令类型互不相同，合成一口就得在 Intake 里先认族再定
				// 形状，装配点从此可以把一族的译装接到另一族的端点上而编译仍绿。
				{Pattern: "/network-catalog-node-registrations", Handler: networkhttp.NewRegisterNodeVersionEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
				{Pattern: "/network-catalog-connection-registrations", Handler: networkhttp.NewRegisterConnectionVersionEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
				{Pattern: "/network-catalog-line-registrations", Handler: networkhttp.NewRegisterLineVersionEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
				{Pattern: "/network-catalog-service-area-registrations", Handler: networkhttp.NewRegisterServiceAreaVersionEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
				{Pattern: "/network-catalog-service-calendar-registrations", Handler: networkhttp.NewRegisterServiceCalendarVersionEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
				{Pattern: "/network-catalog-availability-adjustment-registrations", Handler: networkhttp.NewRegisterAvailabilityAdjustmentEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
				{Pattern: "/network-catalog-route-strategy-registrations", Handler: networkhttp.NewRegisterRouteStrategyVersionEndpoint(channels.operatorRegistries.network, wiring.networkCatalogRegistration)},
			}
		},
	})
}
