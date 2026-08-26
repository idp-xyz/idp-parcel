package main

import (
	"context"
	"errors"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	customsports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	networkdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	networkports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pricingports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	commercialports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	visibilitydomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 各端点构造函数的第二参是应用编排；未配置 Intake 在它之前就拒了，因此本文件这几个
// 类型一个都到不了。它们存在只为回答「到不了的那一格填什么」。
//
// 不填 nil：nil 接口被调会 panic，路由层的 Recoverer 把它兜成一个没有稳定 code 的
// 500，读的人分不出那是进程坏了还是依赖坏了。显式交回错误则落在 ADR-0022 已有的那格
// ——没形成答案，5xx `NO_ANSWER_FORMED`，离线客户端照旧留队重发，而服务端记录里留着
// 下面这句话指明真正的原因。
//
// 它们也不是红线所禁的「开发用」实现：那条禁的是替租户拟一种认证方式、从请求内容铸造
// 来源信封（穿透 ADR-0003）。这几个类型不读请求、不构造任何东西、不作任何业务判断，
// 连一个零值结果都不交回。
//
// 换编排与换 Intake 是两笔可独立发生的工作：运行期未配置 Intake 拒在编排之前，装配期
// 把哪一格换成真编排不动 Intake。提交编排已按审计票 13 经真库与治理桥接真，撤回编排随
// UI 阶段 B 后端序列接真，NO 收寄、TF 交付、VE 索赔与 CC 外部结果四格编排按
// `.scratch/parcel-api-remaining-endpoint-wiring` 的接线票（01、02、04、05）接真，取消
// 编排随第十端点挂载接真（传输层随 UC-PS-006 切片先行落库），运营追踪查阅读口随
// ADR-0076 端点挂载接真库投影库，均由装配点入参交入，不再从本文件取。各格至此全部
// 接真：本文件全部类型只被装配测试用来钉「未配置面」的形状，生产装配没有任何一格再
// 从这里取。真渠道 Intake 就位那笔工作只替换 Intake 本身。
var errOrchestrationNotWired = errors.New("parcel-api: business orchestration is not wired; the unconfigured intake should have refused first")

type unwiredSubmission struct{}

func (unwiredSubmission) Handle(
	context.Context,
	shipmentapp.SubmitShipmentRequestCommand,
) (shipmentapp.SubmitShipmentRequestResult, error) {
	return shipmentapp.SubmitShipmentRequestResult{}, errOrchestrationNotWired
}

type unwiredWithdrawal struct{}

func (unwiredWithdrawal) Handle(
	context.Context,
	shipmentapp.WithdrawShipmentRequestCommand,
) (shipmentapp.WithdrawShipmentRequestResult, error) {
	return shipmentapp.WithdrawShipmentRequestResult{}, errOrchestrationNotWired
}

type unwiredReception struct{}

func (unwiredReception) Handle(
	context.Context,
	nodeopsapp.ReceiveDeliveredUnitCommand,
) (nodeopsapp.ReceiveDeliveredUnitResult, error) {
	return nodeopsapp.ReceiveDeliveredUnitResult{}, errOrchestrationNotWired
}

// unwiredDelivery 一个类型顶两个端点：首登与更正共用 DeliveryHandler 这一个接口。
type unwiredDelivery struct{}

func (unwiredDelivery) Register(
	context.Context,
	tfapp.RegisterEffectiveDeliveryCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	return tfapp.RegisterEffectiveDeliveryResult{}, errOrchestrationNotWired
}

func (unwiredDelivery) Correct(
	context.Context,
	tfapp.CorrectDeliveryProofCommand,
) (tfapp.RegisterEffectiveDeliveryResult, error) {
	return tfapp.RegisterEffectiveDeliveryResult{}, errOrchestrationNotWired
}

// unwiredTrackingViews 只被装配测试使用：生产装配（main）把 VE 真库读适配器交进装配
// 点，这里的占位让「未配置面」测试不必开库。填法照旧：读不回按 ADR-0022 是「没形成
// 答案」的 5xx，绝不能顶成 VIEW_NOT_FOUND——那会把一次进程故障伪装成「查无此件」的
// 终局答案。
type unwiredTrackingViews struct{}

func (unwiredTrackingViews) FindCurrent(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.CustomerAccountReference,
	visibilitydomain.TrackedParcelReference,
) (visibilitydomain.CustomerTrackingView, bool, error) {
	return visibilitydomain.CustomerTrackingView{}, false, errOrchestrationNotWired
}

// unwiredRequestViews 只被装配测试使用：生产装配（main）把真库读适配器交进装配点，
// 这里的占位让「未配置面」测试不必开库。填法同 unwiredTrackingViews——读不回是
// 「没形成答案」的 5xx，绝不顶成一个空列表或统一不可见。
type unwiredRequestViews struct{}

func (unwiredRequestViews) ListVisible(
	context.Context,
	shipmentdomain.AuthorizedQueryScope,
	int,
) ([]shipmentports.ShipmentRequestSummaryRecord, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredRequestViews) FindVisibleByID(
	context.Context,
	shipmentdomain.AuthorizedQueryScope,
	shipmentdomain.ShipmentRequestID,
) (shipmentports.ShipmentRequestDetailRecord, bool, error) {
	return shipmentports.ShipmentRequestDetailRecord{}, false, errOrchestrationNotWired
}

// unwiredProjectionViews 只被装配测试使用：生产装配（main）把真库投影读适配器交进
// 装配点。填法同 unwiredTrackingViews——读不回按 ADR-0022 是「没形成答案」的 5xx，
// 绝不顶成空列表或「投影未形成」。
type unwiredProjectionViews struct{}

func (unwiredProjectionViews) ListCurrent(
	context.Context,
	visibilitydomain.TenantID,
	int,
) ([]visibilitydomain.TrackingProjection, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredProjectionViews) FindCurrent(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.TrackedParcelReference,
) (visibilitydomain.TrackingProjection, bool, error) {
	return visibilitydomain.TrackingProjection{}, false, errOrchestrationNotWired
}

func (unwiredProjectionViews) FindByVersion(
	context.Context,
	visibilitydomain.TenantID,
	visibilitydomain.ProjectionVersionID,
) (visibilitydomain.TrackingProjection, bool, error) {
	return visibilitydomain.TrackingProjection{}, false, errOrchestrationNotWired
}

// 主数据目录的 unwired* 同样只供装配测试。生产 main 为同一上下文的端点交入同一只
// 真库适配器；这里按上下文合成替身，既钉住接口形状，也避免把一次读故障伪装成空目录。
type unwiredPricingCatalogue struct{}

func (unwiredPricingCatalogue) ListPriceCards(
	context.Context,
	pricingdomain.TenantID,
	int,
) ([]pricingports.PriceCardCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredPricingCatalogue) ListReferenceSeries(
	context.Context,
	pricingdomain.TenantID,
	int,
) ([]pricingports.ReferenceSeriesCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredNetworkCatalogue struct{}

func (unwiredNetworkCatalogue) ListNodeVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.NodeDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListConnectionVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.ConnectionDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListLineVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.LineDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListServiceAreaVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.ServiceAreaDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListServiceCalendarVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.ServiceCalendarDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListAvailabilityAdjustments(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.AvailabilityAdjustmentStatement, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredNetworkCatalogue) ListRouteStrategyVersions(
	context.Context,
	networkdomain.TenantID,
	int,
) ([]networkports.RouteStrategyDefinitionVersion, error) {
	return nil, errOrchestrationNotWired
}

type unwiredComplianceRules struct{}

func (unwiredComplianceRules) ListCaseRequirementRules(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.CaseRequirementRuleEntry, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredComplianceRules) ListInterpretationRules(
	context.Context,
	customsdomain.TenantID,
	int,
) ([]customsports.InterpretationRuleEntry, error) {
	return nil, errOrchestrationNotWired
}

type unwiredCommercialCatalogue struct{}

func (unwiredCommercialCatalogue) ListServiceProducts(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.ServiceProductCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListAcceptanceRulePackages(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.AcceptanceRulePackageRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListPreAcceptanceControls(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.PreAcceptanceControlRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListPricePolicies(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.PricePolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListSettlementPolicies(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.SettlementPolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListAsOfPolicyDeclarations(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.AsOfPolicyRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListCustomerContracts(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.CustomerContractCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

func (unwiredCommercialCatalogue) ListSupplierAgreements(
	context.Context,
	commercialdomain.TenantID,
	int,
) ([]commercialports.SupplierAgreementCatalogueRow, error) {
	return nil, errOrchestrationNotWired
}

type unwiredCancellation struct{}

func (unwiredCancellation) Handle(
	context.Context,
	shipmentapp.CancelParcelCommand,
) (shipmentapp.CancelParcelResult, error) {
	return shipmentapp.CancelParcelResult{}, errOrchestrationNotWired
}

type unwiredClaims struct{}

func (unwiredClaims) ReceiveClaim(
	context.Context,
	visibilityapp.ReceiveClaimCommand,
) (visibilityapp.HandleClaimResult, error) {
	return visibilityapp.HandleClaimResult{}, errOrchestrationNotWired
}

type unwiredResults struct{}

func (unwiredResults) Handle(
	context.Context,
	customsapp.ReceiveExternalResultCommand,
) (customsapp.ReceiveExternalResultResult, error) {
	return customsapp.ReceiveExternalResultResult{}, errOrchestrationNotWired
}
