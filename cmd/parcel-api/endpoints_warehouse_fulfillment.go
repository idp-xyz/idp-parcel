package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// warehouseFulfillmentWiring 是仓配履约仓这一册的依赖。enabled 是部署开关（ADR-0375 决定四）的值，从 endpointSources 来、在
// build 里定：为假时其余字段全是零值——关着的部署不构造任何仓配的适配器与编排，mount 也就没有东西可接。
type warehouseFulfillmentWiring struct {
	enabled      bool
	registration warehousehttp.WarehouseRegistrar
	catalogue    warehousehttp.WarehouseCatalogueReader
	revisions    warehousehttp.WarehouseRevisionsReader
}

func init() {
	registerEndpoints(endpointFeature[warehouseFulfillmentWiring]{
		lines: map[string]endpointLine{
			// 登记口挂登记册配置写面，不判准入范围：登记一个修订不形成生产事实（ADR-0100 决定四）。
			"/warehouse-fulfillment-warehouse-registrations": {method: http.MethodPost, face: registryWriteFace(), warehouseFulfillment: true},
			// 读口挂主数据与运营查阅读面。修订史口的仓编码是业务内容、走查询串，探测目标要带上它。
			"/warehouse-fulfillment-warehouses": {method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-warehouse-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-warehouse-revisions?warehouseCode=SYN-WH-01",
			},
		},
		build: func(sources endpointSources) (warehouseFulfillmentWiring, error) {
			if !sources.warehouseFulfillment {
				return warehouseFulfillmentWiring{}, nil
			}
			orchestration, err := buildWarehouseFulfillmentOrchestration(sources.db)
			if err != nil {
				return warehouseFulfillmentWiring{}, err
			}
			return warehouseFulfillmentWiring{
				enabled:      true,
				registration: orchestration.registration,
				catalogue:    orchestration.queries,
				revisions:    orchestration.queries,
			}, nil
		},
		mount: func(channels endpointChannels, wiring warehouseFulfillmentWiring) []httpapi.BusinessEndpoint {
			// 开关关着：各口都是同一类固定处理器，在认证之前答 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED，与渠道配没配无关。恢复动作是
			// 部署方打开开关，不是去配接入渠道，所以不沿用「接入渠道未配置」（ADR-0375 决定四，ADR-0384 决定七）。方法门由那个
			// 处理器按本行声明的方法自守，方法不对的答复与开关开着时同一份。
			registration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			warehouses := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			revisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			// 开着：与别的登记写行、查阅行行为一致——渠道没配答「接入渠道未配置」，配了走真编排。各读口共用一个 Intake。
			if wiring.enabled {
				catalogueIntake := channels.warehouseCatalogue()
				registration = warehousehttp.NewRegisterWarehouseEndpoint(channels.warehouseRegistration(), wiring.registration)
				warehouses = warehousehttp.NewQueryWarehousesEndpoint(catalogueIntake, wiring.catalogue)
				revisions = warehousehttp.NewQueryWarehouseRevisionsEndpoint(catalogueIntake, wiring.revisions)
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/warehouse-fulfillment-warehouse-registrations", Handler: registration},
				{Pattern: "/warehouse-fulfillment-warehouses", Handler: warehouses},
				{Pattern: "/warehouse-fulfillment-warehouse-revisions", Handler: revisions},
			}
		},
	})
}
