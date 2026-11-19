package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
)

// warehouseFulfillmentWiring 是仓配履约各册的依赖：仓、库区、库位类型、库位与包材。enabled 是部署开关（ADR-0375 决定四）的值，从 endpointSources 来、
// 在 build 里定：为假时其余字段全是零值——关着的部署不构造任何仓配的适配器与编排，mount 也就没有东西可接。
//
// 仓配的各册同属一个特性、一个开关、一个依赖结构，不按册拆成几个特性：登记处不许两个特性共用一种依赖结构，而各册的行都带同一个
// 开关，装配测试里「仓配的行」是一个整体（见 warehouseFulfillmentLines）。加一册就是在这里加它的依赖字段、在下面加它的行（ADR-0222）。
type warehouseFulfillmentWiring struct {
	enabled      bool
	registration warehousehttp.WarehouseRegistrar
	catalogue    warehousehttp.WarehouseCatalogueReader
	revisions    warehousehttp.WarehouseRevisionsReader

	zoneRegistration warehousehttp.ZoneRegistrar
	zoneCatalogue    warehousehttp.ZoneCatalogueReader
	zoneRevisions    warehousehttp.ZoneRevisionsReader

	locationTypeRegistration warehousehttp.LocationTypeRegistrar
	locationTypeCatalogue    warehousehttp.LocationTypeCatalogueReader
	locationTypeRevisions    warehousehttp.LocationTypeRevisionsReader

	locationRegistration warehousehttp.LocationRegistrar
	locationCatalogue    warehousehttp.LocationCatalogueReader
	locationRevisions    warehousehttp.LocationRevisionsReader

	packagingMaterialRegistration warehousehttp.PackagingMaterialRegistrar
	packagingMaterialCatalogue    warehousehttp.PackagingMaterialCatalogueReader
	packagingMaterialRevisions    warehousehttp.PackagingMaterialRevisionsReader
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
			// 库区：各行的面与仓同族。库区的身份是（仓编码，库区编码），修订史口的探测目标要带上这两段。
			"/warehouse-fulfillment-zone-registrations": {method: http.MethodPost, face: registryWriteFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-zones":              {method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-zone-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-zone-revisions?warehouseCode=SYN-WH-01&zoneCode=SYN-ZONE-01",
			},
			// 库位类型：各行的面与仓同族。库位类型按租户登记、各仓共用，修订史口的探测目标只带库位类型编码一段。
			"/warehouse-fulfillment-location-type-registrations": {method: http.MethodPost, face: registryWriteFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-location-types":              {method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-location-type-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-location-type-revisions?locationTypeCode=SYN-LT-01",
			},
			// 库位：各行的面与仓同族。库位的身份是（仓编码，库位编码），修订史口的探测目标要带上这两段。
			"/warehouse-fulfillment-location-registrations": {method: http.MethodPost, face: registryWriteFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-locations":              {method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-location-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-location-revisions?warehouseCode=SYN-WH-01&locationCode=SYN-LOC-01",
			},
			// 包材：各行的面与仓同族。包材按租户登记、各仓共用，修订史口的探测目标只带包材编码一段。
			"/warehouse-fulfillment-packaging-material-registrations": {method: http.MethodPost, face: registryWriteFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-packaging-materials":              {method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true},
			"/warehouse-fulfillment-packaging-material-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-packaging-material-revisions?packagingMaterialCode=SYN-BOX-01",
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
				enabled:          true,
				registration:     orchestration.registration,
				catalogue:        orchestration.queries,
				revisions:        orchestration.queries,
				zoneRegistration: orchestration.zoneRegistration,
				zoneCatalogue:    orchestration.zoneQueries,
				zoneRevisions:    orchestration.zoneQueries,

				locationTypeRegistration: orchestration.locationTypeRegistration,
				locationTypeCatalogue:    orchestration.locationTypeQueries,
				locationTypeRevisions:    orchestration.locationTypeQueries,

				locationRegistration: orchestration.locationRegistration,
				locationCatalogue:    orchestration.locationQueries,
				locationRevisions:    orchestration.locationQueries,

				packagingMaterialRegistration: orchestration.packagingMaterialRegistration,
				packagingMaterialCatalogue:    orchestration.packagingMaterialQueries,
				packagingMaterialRevisions:    orchestration.packagingMaterialQueries,
			}, nil
		},
		mount: func(channels endpointChannels, wiring warehouseFulfillmentWiring) []httpapi.BusinessEndpoint {
			// 开关关着：各口都是同一类固定处理器，在认证之前答 403 WAREHOUSE_FULFILLMENT_NOT_ENABLED，与渠道配没配无关。恢复动作是
			// 部署方打开开关，不是去配接入渠道，所以不沿用「接入渠道未配置」（ADR-0375 决定四，ADR-0384 决定七）。方法门由那个
			// 处理器按本行声明的方法自守，方法不对的答复与开关开着时同一份。
			registration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			warehouses := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			revisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			zoneRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			zones := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			zoneRevisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			locationTypeRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			locationTypes := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			locationTypeRevisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			locationRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			locations := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			locationRevisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			packagingMaterialRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			packagingMaterials := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			packagingMaterialRevisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			// 开着：与别的登记写行、查阅行行为一致——渠道没配答「接入渠道未配置」，配了走真编排。各读口共用一个 Intake，各册的
			// 登记口共用同一个认证器。
			if wiring.enabled {
				catalogueIntake := channels.warehouseCatalogue()
				registration = warehousehttp.NewRegisterWarehouseEndpoint(channels.warehouseRegistration(), wiring.registration)
				warehouses = warehousehttp.NewQueryWarehousesEndpoint(catalogueIntake, wiring.catalogue)
				revisions = warehousehttp.NewQueryWarehouseRevisionsEndpoint(catalogueIntake, wiring.revisions)
				zoneRegistration = warehousehttp.NewRegisterZoneEndpoint(channels.zoneRegistration(), wiring.zoneRegistration)
				zones = warehousehttp.NewQueryZonesEndpoint(catalogueIntake, wiring.zoneCatalogue)
				zoneRevisions = warehousehttp.NewQueryZoneRevisionsEndpoint(catalogueIntake, wiring.zoneRevisions)
				locationTypeRegistration = warehousehttp.NewRegisterLocationTypeEndpoint(channels.locationTypeRegistration(), wiring.locationTypeRegistration)
				locationTypes = warehousehttp.NewQueryLocationTypesEndpoint(catalogueIntake, wiring.locationTypeCatalogue)
				locationTypeRevisions = warehousehttp.NewQueryLocationTypeRevisionsEndpoint(catalogueIntake, wiring.locationTypeRevisions)
				locationRegistration = warehousehttp.NewRegisterLocationEndpoint(channels.locationRegistration(), wiring.locationRegistration)
				locations = warehousehttp.NewQueryLocationsEndpoint(catalogueIntake, wiring.locationCatalogue)
				locationRevisions = warehousehttp.NewQueryLocationRevisionsEndpoint(catalogueIntake, wiring.locationRevisions)
				packagingMaterialRegistration = warehousehttp.NewRegisterPackagingMaterialEndpoint(
					channels.packagingMaterialRegistration(), wiring.packagingMaterialRegistration)
				packagingMaterials = warehousehttp.NewQueryPackagingMaterialsEndpoint(catalogueIntake, wiring.packagingMaterialCatalogue)
				packagingMaterialRevisions = warehousehttp.NewQueryPackagingMaterialRevisionsEndpoint(catalogueIntake, wiring.packagingMaterialRevisions)
			}
			return []httpapi.BusinessEndpoint{
				{Pattern: "/warehouse-fulfillment-warehouse-registrations", Handler: registration},
				{Pattern: "/warehouse-fulfillment-warehouses", Handler: warehouses},
				{Pattern: "/warehouse-fulfillment-warehouse-revisions", Handler: revisions},
				{Pattern: "/warehouse-fulfillment-zone-registrations", Handler: zoneRegistration},
				{Pattern: "/warehouse-fulfillment-zones", Handler: zones},
				{Pattern: "/warehouse-fulfillment-zone-revisions", Handler: zoneRevisions},
				{Pattern: "/warehouse-fulfillment-location-type-registrations", Handler: locationTypeRegistration},
				{Pattern: "/warehouse-fulfillment-location-types", Handler: locationTypes},
				{Pattern: "/warehouse-fulfillment-location-type-revisions", Handler: locationTypeRevisions},
				{Pattern: "/warehouse-fulfillment-location-registrations", Handler: locationRegistration},
				{Pattern: "/warehouse-fulfillment-locations", Handler: locations},
				{Pattern: "/warehouse-fulfillment-location-revisions", Handler: locationRevisions},
				{Pattern: "/warehouse-fulfillment-packaging-material-registrations", Handler: packagingMaterialRegistration},
				{Pattern: "/warehouse-fulfillment-packaging-materials", Handler: packagingMaterials},
				{Pattern: "/warehouse-fulfillment-packaging-material-revisions", Handler: packagingMaterialRevisions},
			}
		},
	})
}
