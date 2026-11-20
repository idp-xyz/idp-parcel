package main

import (
	"fmt"
	"net/http"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	whaccess "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/accessidentity"
	warehousehttp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/http"
	warehousedomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
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

	goodsRegistration warehousehttp.GoodsRegistrar
	goodsCatalogue    warehousehttp.GoodsCatalogueReader
	goodsCurrent      warehousehttp.GoodsCurrentReader
	goodsRevisions    warehousehttp.GoodsRevisionsReader
	goodsFinder       warehousehttp.GoodsFinder

	preAdviceOnBehalf warehousehttp.PreAdviceOnBehalf
	preAdviceQueries  warehousehttp.PreAdviceQueries
	preAdviceClose    warehousehttp.PreAdviceCloser

	// customerPreAdvice 缺席时货主六口是字面量未配置。开关关着时这一格不被问到。
	customerPreAdvice *warehousehttp.CustomerInboundPreAdviceIntake
	// customerGoods 缺席时商品货主维护口与商品、仓测货主读口是字面量未配置。开关关着时这一格不被问到。
	customerGoods *warehousehttp.CustomerGoodsIntake
	// customerConclusion 缺席时货主结论口是字面量未配置。开关关着时这一格不被问到。
	customerConclusion    *warehousehttp.CustomerDiscrepancyConclusionIntake
	discrepancyConclusion warehousehttp.DiscrepancyConcluder

	// receiptIntake 缺席时收货确认是字面量未配置。receiptConfirm 是开关开着时的编排；关着时不被问到。
	receiptIntake  *warehousehttp.ReceiptWorkFactIntake
	receiptConfirm warehousehttp.ReceiptConfirmer

	// measurementIntake 缺席时仓测登记是字面量未配置。measurementRecord / measurementQueries 是开关开着时的编排。
	measurementIntake  *warehousehttp.WarehouseMeasurementWorkFactIntake
	measurementRecord  warehousehttp.WarehouseMeasurementRecorder
	measurementQueries warehousehttp.WarehouseMeasurementReader

	disposalRuleRegistration warehousehttp.DisposalRuleRegistrar
	disposalRuleCatalogue    warehousehttp.DisposalRuleCatalogueReader
	disposalRuleRevisions    warehousehttp.DisposalRuleRevisionsReader
	unclaimedArrivalIntake   *warehousehttp.UnclaimedArrivalWorkFactIntake
	unclaimedArrivalRegister warehousehttp.UnclaimedArrivalRegistrar
	unclaimedArrivalManage   warehousehttp.UnclaimedArrivalManager
	unclaimedArrivalQueries  warehousehttp.UnclaimedArrivalQueries

	internalMoveIntake        *warehousehttp.InternalJobWorkFactIntake
	formConversionIntake      *warehousehttp.InternalJobWorkFactIntake
	conditionConversionIntake *warehousehttp.InternalJobWorkFactIntake
	shortageIntake            *warehousehttp.InternalJobWorkFactIntake
	internalJobs              warehousehttp.InternalJobRegistrar
	conditionRestore          warehousehttp.ConditionRestorer
	internalJobQueries        warehousehttp.InternalJobQueries
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
			// 商品代录挂运营决定，要判准入范围。读口挂主数据与运营查阅读，不判准入范围；货主是查询串里的业务内容。
			"/warehouse-fulfillment-goods-on-behalf": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionGoodsRecordOnBehalf), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-goods": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-goods?ownerAccount=SYN-OWNER-01",
			},
			"/warehouse-fulfillment-goods-current": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-goods-current?ownerAccount=SYN-OWNER-01&goodsCode=SYN-GOODS-01",
			},
			"/warehouse-fulfillment-goods-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-goods-revisions?ownerAccount=SYN-OWNER-01&goodsCode=SYN-GOODS-01",
			},
			"/warehouse-fulfillment-goods-by-code": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-goods-by-code?ownerAccount=SYN-OWNER-01&code=SYN-GOODS-01",
			},
			// 入库预报代录三个写口挂同一种运营决定，都判准入。读口挂主数据与运营查阅读，不判准入。一路径一行。
			"/warehouse-fulfillment-inbound-pre-advice-on-behalf": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionInboundPreAdviceRecordOnBehalf), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-inbound-pre-advice-notice-on-behalf": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionInboundPreAdviceRecordOnBehalf), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionInboundPreAdviceRecordOnBehalf), warehouseFulfillment: true,
			},
			// 入库预报完结挂运营决定，不挂作业事实登记。准入的能力词与事实类型词都是这一种决定的字面量。
			"/warehouse-fulfillment-inbound-pre-advice-closures": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionInboundPreAdviceClose), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-inbound-pre-advices": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-inbound-pre-advices?ownerAccount=SYN-OWNER-01",
			},
			"/warehouse-fulfillment-inbound-pre-advice": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-inbound-pre-advice?ownerAccount=SYN-OWNER-01&preAdviceId=SYN-IPA-01",
			},
			"/warehouse-fulfillment-inbound-pre-advice-by-number": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-inbound-pre-advice-by-number?documentNumber=IB-20261120-00001",
			},
			// 货主六口挂首方客户渠道，Intake 由本特性从客户铸造器自建。写口与读口都在这份登记里，开关关着时仍是
			// WAREHOUSE_FULFILLMENT_NOT_ENABLED，在认证之前。读口是契约「落端点时按对象拆开」的那三行，能力词不拆。
			"/v1/warehouse/inbound-pre-advices": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityInboundPreAdviceSubmission),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/v1/warehouse/inbound-pre-advices/arrival-notices": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityInboundPreAdviceArrivalNotice),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/v1/warehouse/inbound-pre-advices/withdrawals": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityInboundPreAdviceWithdrawal),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/v1/warehouse/reads/inbound-pre-advices": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/v1/warehouse/reads/inbound-pre-advice": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/inbound-pre-advice?preAdviceId=SYN-IPA-01",
			},
			"/v1/warehouse/reads/inbound-pre-advice-by-number": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/inbound-pre-advice-by-number?documentNumber=IB-20261120-00001",
			},
			// 收货差异结论代录挂运营决定。准入的能力词与事实类型词都是这一种决定的字面量。不带决定标识。
			"/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionReceiptDiscrepancyRecordOnBehalf), warehouseFulfillment: true,
			},
			// 货主结论口挂首方客户渠道。准入两格都是这一口的字面量，不用 CUSTOMER_COMMAND。开关关着时在认证之前答未启用。
			"/v1/warehouse/receipt-discrepancy-conclusions": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityReceiptDiscrepancyConclusion),
				ownIntake: true, warehouseFulfillment: true,
			},
			// 收货确认挂作业事实登记，不挂运营决定。铸造侧判准入：能力词是作业事实登记，事实类型是收货确认。
			// Intake 由本特性自建，不经节点收寄那一只——那一只只认 NODE。
			"/warehouse-fulfillment-receipt-confirmations": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			// 商品货主维护口与货主商品、仓测读口挂首方客户渠道，Intake 由本特性自建。
			"/v1/warehouse/goods": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityGoodsMaintenance),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/v1/warehouse/reads/goods": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/v1/warehouse/reads/goods-current": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/goods-current?goodsCode=SYN-GOODS-01",
			},
			"/v1/warehouse/reads/goods-revisions": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/goods-revisions?goodsCode=SYN-GOODS-01",
			},
			"/v1/warehouse/reads/goods-by-code": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/goods-by-code?code=SYN-GOODS-01",
			},
			"/v1/warehouse/reads/goods-measurements": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/goods-measurements?goodsCode=SYN-GOODS-01&packagingLevel=EA",
			},
			"/v1/warehouse/reads/carton-measurements": {
				method: http.MethodGet, face: customerClientFace(accessidentity.CustomerCapabilityWarehouseRead),
				ownIntake: true, warehouseFulfillment: true,
				probe: "/v1/warehouse/reads/carton-measurements?warehouseCode=SYN-WH-01&cartonNumber=SYN-CARTON-01",
			},
			// 仓测登记挂作业事实登记（范围种类 WAREHOUSE，事实类型 WAREHOUSE_MEASUREMENT）；运营查阅挂主数据与运营查阅读。
			"/warehouse-fulfillment-warehouse-measurements": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-goods-measurements": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-goods-measurements?ownerAccount=SYN-OWNER-01&goodsCode=SYN-GOODS-01&packagingLevel=EA",
			},
			"/warehouse-fulfillment-carton-measurements": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-carton-measurements?ownerAccount=SYN-OWNER-01&warehouseCode=SYN-WH-01&cartonNumber=SYN-CARTON-01",
			},
			// 待认领到货处置规则挂登记册配置写，不判准入（ADR-0384 决定五）；查阅挂主数据与运营查阅读。
			"/warehouse-fulfillment-unclaimed-disposal-rule-registrations": {
				method: http.MethodPost, face: registryWriteFace(), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-disposal-rules": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-disposal-rule-revisions": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-unclaimed-disposal-rule-revisions?warehouseCode=SYN-WH-01",
			},
			// 待认领到货直接登记挂作业事实登记（范围种类 WAREHOUSE，事实类型 UNCLAIMED_ARRIVAL）；认领、撤销与处置挂运营决定；
			// 查阅只在操作者面，不上客户面（UC-WF-005、BD-WF-012）。
			"/warehouse-fulfillment-unclaimed-arrival-registrations": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-arrival-claims": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionUnclaimedArrivalClaim), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-arrival-claim-revocations": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionUnclaimedArrivalClaim), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-arrival-dispositions": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionUnclaimedArrivalDisposition), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-arrivals": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-unclaimed-arrival": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-unclaimed-arrival?arrivalId=SYN-UCA-01",
			},
			// 库内作业四个登记口挂作业事实登记（范围种类 WAREHOUSE）。上架、移库与补货都是 INTERNAL_MOVE。
			"/warehouse-fulfillment-internal-moves": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-form-conversions": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-condition-conversions": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-shortage-registrations": {
				method: http.MethodPost, face: workFactFace(accessidentity.WorkScopeWarehouse),
				ownIntake: true, warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-condition-restores": {
				method: http.MethodPost, face: decisionFace(accessidentity.DecisionConditionRestore), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-internal-jobs": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
			},
			"/warehouse-fulfillment-internal-job": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-internal-job?factId=SYN-FACT-01",
			},
			"/warehouse-fulfillment-condition-restore": {
				method: http.MethodGet, face: operationsReadFace(), warehouseFulfillment: true,
				probe: "/warehouse-fulfillment-condition-restore?decisionId=SYN-DEC-01",
			},
		},
		build: func(sources endpointSources) (warehouseFulfillmentWiring, error) {
			if !sources.warehouseFulfillment {
				return warehouseFulfillmentWiring{}, nil
			}
			orchestration, err := buildWarehouseFulfillmentOrchestration(sources.db, sources.documentNumbers.Assigner)
			if err != nil {
				return warehouseFulfillmentWiring{}, err
			}
			var customerPreAdvice *warehousehttp.CustomerInboundPreAdviceIntake
			var customerGoods *warehousehttp.CustomerGoodsIntake
			var customerConclusion *warehousehttp.CustomerDiscrepancyConclusionIntake
			if sources.customerClientMinter != nil {
				customerPreAdvice, err = buildCustomerInboundPreAdviceIntake(sources.customerClientMinter)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
				customerGoods, err = buildCustomerGoodsIntake(sources.customerClientMinter)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
				customerConclusion, err = buildCustomerDiscrepancyConclusionIntake(sources.customerClientMinter)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
			}
			var receiptIntake *warehousehttp.ReceiptWorkFactIntake
			var measurementIntake *warehousehttp.WarehouseMeasurementWorkFactIntake
			var unclaimedArrivalIntake *warehousehttp.UnclaimedArrivalWorkFactIntake
			var internalMoveIntake *warehousehttp.InternalJobWorkFactIntake
			var formConversionIntake *warehousehttp.InternalJobWorkFactIntake
			var conditionConversionIntake *warehousehttp.InternalJobWorkFactIntake
			var shortageIntake *warehousehttp.InternalJobWorkFactIntake
			if sources.operatorVerifier != nil && sources.operatorRegistry != nil && sources.deviceRegistry != nil && sources.admission != nil {
				receiptIntake, err = buildReceiptConfirmationIntake(
					sources.operatorVerifier, sources.operatorRegistry, sources.deviceRegistry, sources.admission,
				)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
				measurementIntake, err = buildWarehouseMeasurementIntake(
					sources.operatorVerifier, sources.operatorRegistry, sources.deviceRegistry, sources.admission,
				)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
				unclaimedArrivalIntake, err = buildUnclaimedArrivalIntake(
					sources.operatorVerifier, sources.operatorRegistry, sources.deviceRegistry, sources.admission,
				)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
				internalMoveIntake, formConversionIntake, conditionConversionIntake, shortageIntake, err = buildInternalJobIntakes(
					sources.operatorVerifier, sources.operatorRegistry, sources.deviceRegistry, sources.admission,
				)
				if err != nil {
					return warehouseFulfillmentWiring{}, err
				}
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

				goodsRegistration: orchestration.goodsRegistration,
				goodsCatalogue:    orchestration.goodsQueries,
				goodsCurrent:      orchestration.goodsQueries,
				goodsRevisions:    orchestration.goodsQueries,
				goodsFinder:       orchestration.goodsQueries,

				preAdviceOnBehalf:         orchestration.preAdvices,
				preAdviceQueries:          orchestration.preAdviceQueries,
				preAdviceClose:            orchestration.preAdviceClose,
				customerPreAdvice:         customerPreAdvice,
				customerGoods:             customerGoods,
				customerConclusion:        customerConclusion,
				discrepancyConclusion:     orchestration.discrepancyConclusion,
				receiptIntake:             receiptIntake,
				receiptConfirm:            orchestration.receiptConfirm,
				measurementIntake:         measurementIntake,
				measurementRecord:         orchestration.measurementRecord,
				measurementQueries:        orchestration.measurementQueries,
				disposalRuleRegistration:  orchestration.disposalRuleRegistration,
				disposalRuleCatalogue:     orchestration.disposalRuleQueries,
				disposalRuleRevisions:     orchestration.disposalRuleQueries,
				unclaimedArrivalIntake:    unclaimedArrivalIntake,
				unclaimedArrivalRegister:  orchestration.unclaimedArrivalRegister,
				unclaimedArrivalManage:    orchestration.unclaimedArrivalManage,
				unclaimedArrivalQueries:   orchestration.unclaimedArrivalQueries,
				internalMoveIntake:        internalMoveIntake,
				formConversionIntake:      formConversionIntake,
				conditionConversionIntake: conditionConversionIntake,
				shortageIntake:            shortageIntake,
				internalJobs:              orchestration.internalJobs,
				conditionRestore:          orchestration.internalJobs,
				internalJobQueries:        orchestration.internalJobQueries,
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
			goodsOnBehalf := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			goods := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			goodsCurrent := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			goodsRevisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			goodsByCode := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			preAdviceOnBehalf := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			preAdviceNotice := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			preAdviceWithdrawal := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			preAdviceClose := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			preAdvices := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			preAdvice := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			preAdviceByNumber := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			customerSubmission := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodPost))
			customerNotice := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodPost))
			customerWithdrawal := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodPost))
			customerList := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerOne := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerByNumber := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerConclusion := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodPost))
			discrepancyConclusion := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			receiptConfirmation := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			customerGoodsWrite := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodPost))
			customerGoodsList := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerGoodsCurrent := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerGoodsRevisions := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerGoodsByCode := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerGoodsMeasurements := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			customerCartonMeasurements := http.Handler(warehousehttp.NewCustomerNotEnabledEndpoint(http.MethodGet))
			warehouseMeasurement := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			goodsMeasurements := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			cartonMeasurements := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			disposalRuleRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			disposalRules := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			disposalRuleRevisions := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			unclaimedArrivalRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			unclaimedArrivalClaim := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			unclaimedArrivalRevoke := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			unclaimedArrivalDispose := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			unclaimedArrivalList := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			unclaimedArrivalOne := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			internalMove := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			formConversion := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			conditionConversion := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			shortageRegistration := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			conditionRestore := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodPost))
			internalJobList := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			internalJobOne := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
			conditionRestoreOne := http.Handler(warehousehttp.NewNotEnabledEndpoint(http.MethodGet))
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
				goodsOnBehalf = warehousehttp.NewRegisterGoodsOnBehalfEndpoint(channels.goodsOnBehalf(), wiring.goodsRegistration)
				goods = warehousehttp.NewQueryGoodsEndpoint(catalogueIntake, wiring.goodsCatalogue)
				goodsCurrent = warehousehttp.NewQueryCurrentGoodsEndpoint(catalogueIntake, wiring.goodsCurrent)
				goodsRevisions = warehousehttp.NewQueryGoodsRevisionsEndpoint(catalogueIntake, wiring.goodsRevisions)
				goodsByCode = warehousehttp.NewFindGoodsEndpoint(catalogueIntake, wiring.goodsFinder)
				preAdviceOnBehalf = warehousehttp.NewAcceptPreAdviceOnBehalfEndpoint(channels.preAdviceOnBehalf(), wiring.preAdviceOnBehalf)
				preAdviceNotice = warehousehttp.NewUpdatePreAdviceNoticeOnBehalfEndpoint(channels.preAdviceOnBehalf(), wiring.preAdviceOnBehalf)
				preAdviceWithdrawal = warehousehttp.NewWithdrawPreAdviceOnBehalfEndpoint(channels.preAdviceOnBehalf(), wiring.preAdviceOnBehalf)
				preAdviceClose = warehousehttp.NewClosePreAdviceEndpoint(channels.preAdviceClose(), wiring.preAdviceClose)
				preAdvices = warehousehttp.NewQueryPreAdvicesEndpoint(catalogueIntake, wiring.preAdviceQueries)
				preAdvice = warehousehttp.NewQueryPreAdviceEndpoint(catalogueIntake, wiring.preAdviceQueries)
				preAdviceByNumber = warehousehttp.NewQueryPreAdviceByNumberEndpoint(catalogueIntake, wiring.preAdviceQueries)
				customerPorts := warehousehttp.OpenCustomerInboundPreAdviceIntake(wiring.customerPreAdvice)
				customerSubmission = warehousehttp.NewSubmitInboundPreAdviceEndpoint(customerPorts, wiring.preAdviceOnBehalf)
				customerNotice = warehousehttp.NewUpdateInboundPreAdviceNoticeEndpoint(customerPorts, wiring.preAdviceOnBehalf)
				customerWithdrawal = warehousehttp.NewWithdrawInboundPreAdviceEndpoint(customerPorts, wiring.preAdviceOnBehalf)
				customerList = warehousehttp.NewListCustomerInboundPreAdvicesEndpoint(customerPorts, wiring.preAdviceQueries)
				customerOne = warehousehttp.NewGetCustomerInboundPreAdviceEndpoint(customerPorts, wiring.preAdviceQueries)
				customerByNumber = warehousehttp.NewGetCustomerInboundPreAdviceByNumberEndpoint(customerPorts, wiring.preAdviceQueries)
				conclusionPorts := warehousehttp.OpenCustomerDiscrepancyConclusionIntake(wiring.customerConclusion)
				customerConclusion = warehousehttp.NewConcludeDiscrepancyEndpoint(conclusionPorts, wiring.discrepancyConclusion)
				discrepancyConclusion = warehousehttp.NewConcludeDiscrepancyOnBehalfEndpoint(channels.discrepancyConclusion(), wiring.discrepancyConclusion)
				receiptIntake := warehousehttp.ReceiptConfirmationIntake(warehousehttp.UnconfiguredReceiptIntake{})
				if wiring.receiptIntake != nil {
					receiptIntake = wiring.receiptIntake
				}
				receiptConfirmation = warehousehttp.NewConfirmReceiptEndpoint(receiptIntake, wiring.receiptConfirm)
				goodsPorts := warehousehttp.OpenCustomerGoodsIntake(wiring.customerGoods)
				customerGoodsWrite = warehousehttp.NewMaintainCustomerGoodsEndpoint(goodsPorts, wiring.goodsRegistration)
				customerGoodsList = warehousehttp.NewListCustomerGoodsEndpoint(goodsPorts, wiring.goodsCatalogue)
				customerGoodsCurrent = warehousehttp.NewGetCustomerCurrentGoodsEndpoint(goodsPorts, wiring.goodsCurrent)
				customerGoodsRevisions = warehousehttp.NewListCustomerGoodsRevisionsEndpoint(goodsPorts, wiring.goodsRevisions)
				customerGoodsByCode = warehousehttp.NewFindCustomerGoodsEndpoint(goodsPorts, wiring.goodsFinder)
				customerGoodsMeasurements = warehousehttp.NewGetCustomerGoodsMeasurementsEndpoint(goodsPorts, wiring.measurementQueries)
				customerCartonMeasurements = warehousehttp.NewGetCustomerCartonMeasurementsEndpoint(goodsPorts, wiring.measurementQueries)
				measurementIntake := warehousehttp.WarehouseMeasurementIntake(warehousehttp.UnconfiguredWarehouseMeasurementIntake{})
				if wiring.measurementIntake != nil {
					measurementIntake = wiring.measurementIntake
				}
				warehouseMeasurement = warehousehttp.NewRecordWarehouseMeasurementEndpoint(measurementIntake, wiring.measurementRecord)
				goodsMeasurements = warehousehttp.NewQueryGoodsMeasurementsEndpoint(catalogueIntake, wiring.measurementQueries)
				cartonMeasurements = warehousehttp.NewQueryCartonMeasurementsEndpoint(catalogueIntake, wiring.measurementQueries)
				disposalRuleRegistration = warehousehttp.NewRegisterDisposalRuleEndpoint(channels.unclaimedDisposalRuleRegistration(), wiring.disposalRuleRegistration)
				disposalRules = warehousehttp.NewQueryDisposalRulesEndpoint(catalogueIntake, wiring.disposalRuleCatalogue)
				disposalRuleRevisions = warehousehttp.NewQueryDisposalRuleRevisionsEndpoint(catalogueIntake, wiring.disposalRuleRevisions)
				unclaimedIntake := warehousehttp.UnclaimedArrivalWorkFactIntakeFace(warehousehttp.UnconfiguredUnclaimedArrivalWorkFactIntake{})
				if wiring.unclaimedArrivalIntake != nil {
					unclaimedIntake = wiring.unclaimedArrivalIntake
				}
				unclaimedArrivalRegistration = warehousehttp.NewRegisterUnclaimedArrivalEndpoint(unclaimedIntake, wiring.unclaimedArrivalRegister)
				unclaimedArrivalClaim = warehousehttp.NewClaimUnclaimedArrivalEndpoint(channels.unclaimedArrivalClaim(), wiring.unclaimedArrivalManage)
				unclaimedArrivalRevoke = warehousehttp.NewRevokeUnclaimedClaimEndpoint(channels.unclaimedArrivalClaim(), wiring.unclaimedArrivalManage)
				unclaimedArrivalDispose = warehousehttp.NewDisposeUnclaimedArrivalEndpoint(channels.unclaimedArrivalDisposition(), wiring.unclaimedArrivalManage)
				unclaimedArrivalList = warehousehttp.NewQueryUnclaimedArrivalsEndpoint(catalogueIntake, wiring.unclaimedArrivalQueries)
				unclaimedArrivalOne = warehousehttp.NewQueryUnclaimedArrivalEndpoint(catalogueIntake, wiring.unclaimedArrivalQueries)
				internalMoveIntake := warehousehttp.UnconfiguredInternalJobIntake{}
				if wiring.internalMoveIntake != nil {
					internalMove = warehousehttp.NewRegisterInternalJobEndpoint(wiring.internalMoveIntake, wiring.internalJobs)
				} else {
					internalMove = warehousehttp.NewRegisterInternalJobEndpoint(internalMoveIntake, wiring.internalJobs)
				}
				formIntake := warehousehttp.UnconfiguredInternalJobIntake{}
				if wiring.formConversionIntake != nil {
					formConversion = warehousehttp.NewRegisterInternalJobEndpoint(wiring.formConversionIntake, wiring.internalJobs)
				} else {
					formConversion = warehousehttp.NewRegisterInternalJobEndpoint(formIntake, wiring.internalJobs)
				}
				conditionIntake := warehousehttp.UnconfiguredInternalJobIntake{}
				if wiring.conditionConversionIntake != nil {
					conditionConversion = warehousehttp.NewRegisterInternalJobEndpoint(wiring.conditionConversionIntake, wiring.internalJobs)
				} else {
					conditionConversion = warehousehttp.NewRegisterInternalJobEndpoint(conditionIntake, wiring.internalJobs)
				}
				shortageFactIntake := warehousehttp.UnconfiguredInternalJobIntake{}
				if wiring.shortageIntake != nil {
					shortageRegistration = warehousehttp.NewRegisterInternalJobEndpoint(wiring.shortageIntake, wiring.internalJobs)
				} else {
					shortageRegistration = warehousehttp.NewRegisterInternalJobEndpoint(shortageFactIntake, wiring.internalJobs)
				}
				conditionRestore = warehousehttp.NewRestoreConditionEndpoint(channels.conditionRestore(), wiring.conditionRestore)
				internalJobList = warehousehttp.NewQueryInternalJobsEndpoint(catalogueIntake, wiring.internalJobQueries)
				internalJobOne = warehousehttp.NewQueryInternalJobEndpoint(catalogueIntake, wiring.internalJobQueries)
				conditionRestoreOne = warehousehttp.NewQueryConditionRestoreEndpoint(catalogueIntake, wiring.internalJobQueries)
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
				{Pattern: "/warehouse-fulfillment-goods-on-behalf", Handler: goodsOnBehalf},
				{Pattern: "/warehouse-fulfillment-goods", Handler: goods},
				{Pattern: "/warehouse-fulfillment-goods-current", Handler: goodsCurrent},
				{Pattern: "/warehouse-fulfillment-goods-revisions", Handler: goodsRevisions},
				{Pattern: "/warehouse-fulfillment-goods-by-code", Handler: goodsByCode},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advice-on-behalf", Handler: preAdviceOnBehalf},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advice-notice-on-behalf", Handler: preAdviceNotice},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advice-withdrawal-on-behalf", Handler: preAdviceWithdrawal},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advice-closures", Handler: preAdviceClose},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advices", Handler: preAdvices},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advice", Handler: preAdvice},
				{Pattern: "/warehouse-fulfillment-inbound-pre-advice-by-number", Handler: preAdviceByNumber},
				{Pattern: "/v1/warehouse/inbound-pre-advices", Handler: customerSubmission},
				{Pattern: "/v1/warehouse/inbound-pre-advices/arrival-notices", Handler: customerNotice},
				{Pattern: "/v1/warehouse/inbound-pre-advices/withdrawals", Handler: customerWithdrawal},
				{Pattern: "/v1/warehouse/reads/inbound-pre-advices", Handler: customerList},
				{Pattern: "/v1/warehouse/reads/inbound-pre-advice", Handler: customerOne},
				{Pattern: "/v1/warehouse/reads/inbound-pre-advice-by-number", Handler: customerByNumber},
				{Pattern: "/v1/warehouse/receipt-discrepancy-conclusions", Handler: customerConclusion},
				{Pattern: "/warehouse-fulfillment-receipt-discrepancy-conclusion-on-behalf", Handler: discrepancyConclusion},
				{Pattern: "/warehouse-fulfillment-receipt-confirmations", Handler: receiptConfirmation},
				{Pattern: "/v1/warehouse/goods", Handler: customerGoodsWrite},
				{Pattern: "/v1/warehouse/reads/goods", Handler: customerGoodsList},
				{Pattern: "/v1/warehouse/reads/goods-current", Handler: customerGoodsCurrent},
				{Pattern: "/v1/warehouse/reads/goods-revisions", Handler: customerGoodsRevisions},
				{Pattern: "/v1/warehouse/reads/goods-by-code", Handler: customerGoodsByCode},
				{Pattern: "/v1/warehouse/reads/goods-measurements", Handler: customerGoodsMeasurements},
				{Pattern: "/v1/warehouse/reads/carton-measurements", Handler: customerCartonMeasurements},
				{Pattern: "/warehouse-fulfillment-warehouse-measurements", Handler: warehouseMeasurement},
				{Pattern: "/warehouse-fulfillment-goods-measurements", Handler: goodsMeasurements},
				{Pattern: "/warehouse-fulfillment-carton-measurements", Handler: cartonMeasurements},
				{Pattern: "/warehouse-fulfillment-unclaimed-disposal-rule-registrations", Handler: disposalRuleRegistration},
				{Pattern: "/warehouse-fulfillment-unclaimed-disposal-rules", Handler: disposalRules},
				{Pattern: "/warehouse-fulfillment-unclaimed-disposal-rule-revisions", Handler: disposalRuleRevisions},
				{Pattern: "/warehouse-fulfillment-unclaimed-arrival-registrations", Handler: unclaimedArrivalRegistration},
				{Pattern: "/warehouse-fulfillment-unclaimed-arrival-claims", Handler: unclaimedArrivalClaim},
				{Pattern: "/warehouse-fulfillment-unclaimed-arrival-claim-revocations", Handler: unclaimedArrivalRevoke},
				{Pattern: "/warehouse-fulfillment-unclaimed-arrival-dispositions", Handler: unclaimedArrivalDispose},
				{Pattern: "/warehouse-fulfillment-unclaimed-arrivals", Handler: unclaimedArrivalList},
				{Pattern: "/warehouse-fulfillment-unclaimed-arrival", Handler: unclaimedArrivalOne},
				{Pattern: "/warehouse-fulfillment-internal-moves", Handler: internalMove},
				{Pattern: "/warehouse-fulfillment-form-conversions", Handler: formConversion},
				{Pattern: "/warehouse-fulfillment-condition-conversions", Handler: conditionConversion},
				{Pattern: "/warehouse-fulfillment-shortage-registrations", Handler: shortageRegistration},
				{Pattern: "/warehouse-fulfillment-condition-restores", Handler: conditionRestore},
				{Pattern: "/warehouse-fulfillment-internal-jobs", Handler: internalJobList},
				{Pattern: "/warehouse-fulfillment-internal-job", Handler: internalJobOne},
				{Pattern: "/warehouse-fulfillment-condition-restore", Handler: conditionRestoreOne},
			}
		},
	})
}

// buildCustomerInboundPreAdviceIntake 装配入库预报货主口的首方 Intake。铸造器与别的首方口同一只。铸造器缺席时不调用本函数，
// 六口保持字面量未配置。
func buildCustomerDiscrepancyConclusionIntake(minter *accessidentity.CustomerClientMinter) (*warehousehttp.CustomerDiscrepancyConclusionIntake, error) {
	authenticator, err := whaccess.NewCustomerDiscrepancyConclusionAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return warehousehttp.NewCustomerDiscrepancyConclusionIntake(authenticator)
}

func buildCustomerInboundPreAdviceIntake(minter *accessidentity.CustomerClientMinter) (*warehousehttp.CustomerInboundPreAdviceIntake, error) {
	authenticator, err := whaccess.NewCustomerInboundPreAdviceAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return warehousehttp.NewCustomerInboundPreAdviceIntake(authenticator)
}

func buildCustomerGoodsIntake(minter *accessidentity.CustomerClientMinter) (*warehousehttp.CustomerGoodsIntake, error) {
	authenticator, err := whaccess.NewCustomerGoodsAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return warehousehttp.NewCustomerGoodsIntake(authenticator)
}

// buildReceiptConfirmationIntake 装配收货确认的作业事实 Intake。铸造器与收寄口同一组核验方、操作者册、设备册与准入范围，分开建。
// 四件缺一就不调用本函数，这一口保持字面量未配置。
func buildReceiptConfirmationIntake(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (*warehousehttp.ReceiptWorkFactIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: receipt confirmation work fact minter: %w", err)
	}
	authenticator, err := whaccess.NewReceiptConfirmationAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return warehousehttp.NewReceiptWorkFactIntake(authenticator)
}

// buildWarehouseMeasurementIntake 装配仓测登记的作业事实 Intake。
func buildWarehouseMeasurementIntake(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (*warehousehttp.WarehouseMeasurementWorkFactIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: warehouse measurement work fact minter: %w", err)
	}
	authenticator, err := whaccess.NewWarehouseMeasurementAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return warehousehttp.NewWarehouseMeasurementWorkFactIntake(authenticator)
}

// buildUnclaimedArrivalIntake 装配待认领到货登记的作业事实 Intake。
func buildUnclaimedArrivalIntake(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (*warehousehttp.UnclaimedArrivalWorkFactIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: unclaimed arrival work fact minter: %w", err)
	}
	authenticator, err := whaccess.NewUnclaimedArrivalWorkFactAuthenticator(minter)
	if err != nil {
		return nil, err
	}
	return warehousehttp.NewUnclaimedArrivalWorkFactIntake(authenticator)
}

// buildInternalJobIntakes 装配库内移动、形态转换、状态转换与短少四个作业事实 Intake。四口共用一只铸造器，事实种类各绑一个。
func buildInternalJobIntakes(
	verifier accessidentity.OperatorCredentialVerifier,
	operators accessidentity.OperatorRegistry,
	devices accessidentity.DeviceRegistry,
	admission accessidentity.AdmissionScope,
) (*warehousehttp.InternalJobWorkFactIntake, *warehousehttp.InternalJobWorkFactIntake, *warehousehttp.InternalJobWorkFactIntake, *warehousehttp.InternalJobWorkFactIntake, error) {
	minter, err := accessidentity.NewWorkFactMinter(verifier, operators, devices, admission, time.Now)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parcel-api: internal job work fact minter: %w", err)
	}
	build := func(kind accessidentity.WorkFactKind, domainKind warehousedomain.InternalJobKind) (*warehousehttp.InternalJobWorkFactIntake, error) {
		authenticator, err := whaccess.NewInternalJobAuthenticator(minter, kind)
		if err != nil {
			return nil, err
		}
		return warehousehttp.NewInternalJobWorkFactIntake(authenticator, domainKind)
	}
	move, err := build(accessidentity.WorkFactInternalMove, warehousedomain.InternalJobMove)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	form, err := build(accessidentity.WorkFactFormConversion, warehousedomain.InternalJobForm)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	condition, err := build(accessidentity.WorkFactConditionConversion, warehousedomain.InternalJobCondition)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	shortage, err := build(accessidentity.WorkFactShortage, warehousedomain.InternalJobShortage)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return move, form, condition, shortage, nil
}
