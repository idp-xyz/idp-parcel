package main

import (
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	networkhttp "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/http"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// assembleBusinessEndpoints 是业务端点的装配点（组合根）。各上下文 adapters/http
// 里的接入面处理器全部挂在这里：命令面、业务查阅面与主数据目录查阅面都从这一处进入
// 进程路由，领域边界仍由各自的处理器和读口保持。
//
// 按 ADR-0055，本函数不再以空清单等 `PAR-INT-01`：每个端点各以「未配置即拒」的 Intake
// 起步——不读业务内容、不采信自报身份、不构造命令，对每个请求如实答「接入渠道未配置」
// （403 + ACCESS_CHANNEL_NOT_CONFIGURED）。空清单折叠了两件事：进程外看「产品没有这个
// 能力」与「租户还没配置接入渠道」同答 404，而前者无事可做、后者要去提供渠道参数。
//
// 红线不因此松动：这里不得出现任何「开发用」的采信头部实现。未配置即拒不是那种默认
// 实现——分界同 ADR-0052：「读一个空登记册并如实答未配置不是默认实现，恰恰是它想保护
// 的东西」，而这里的空登记册就是本函数自己：真渠道就位前它没有任何一行真 Intake。
//
// 真渠道 Intake 就位时在本函数逐端点替换，路由层与处理器不动；载荷规范化摘要与准入
// 范围装配（`PAR-GOV-03..07`）仍拦着真渠道 Intake，未配置即拒绕开它们只因它走不到那
// 一步（ADR-0055 第五条）。
//
// 路径取各包传输层测试已在用的那一个，不另立一套坐标；TF 的 POD 更正此前没有自己的
// 路径，按它与首登「命令形状与恢复动作不同、故分两个端点」的理由取独立子资源。这些
// 路径今天还不是任何租户的对外契约——真渠道就位那笔工作若要改，改的是本函数一处。
//
// 各端点的第二参（应用编排或读口）与 Intake 是两笔独立的接线：命令面编排均经真库，
// 委托、追踪与主数据目录查阅直接消费所属上下文的真库存储读面，全部由 main 构造后入参
// 交入。unwired* 类型只余装配测试在用，分辨见 unwired_orchestration.go 的文件注释。
//
// 主数据目录查阅按 ADR-0077 各自消费所属上下文存储；网络目录与服务区域共用
// /network-catalog 的 family 分派，其余页面各有独立入口。此处按入口逐项枚举，少装一个
// 就是把它折回 404，与「渠道未配置」不可分辨，那正是 ADR-0055 要治的病。
//
// 所有 *Views、*Catalog 与 *Rules 参数都是查阅端点的读口：读面不是编排（查阅不触发
// 判断、派生或披露），生产装配交入各自的真库读适配器；未配置 Intake 仍拒在它们之前，
// 接入渠道就位前一次也不会被调到。
func assembleBusinessEndpoints(
	submission shipmenthttp.SubmissionHandler,
	withdrawal shipmenthttp.WithdrawalHandler,
	requestViews shipmenthttp.ShipmentRequestViewsReader,
	cancellation shipmenthttp.CancellationHandler,
	reception nodeopshttp.ReceptionHandler,
	delivery tfhttp.DeliveryHandler,
	trackingViews visibilityhttp.TrackingViewReader,
	projectionViews visibilityhttp.OperationsProjectionReader,
	claims visibilityhttp.ClaimReceiver,
	results customshttp.ResultHandler,
	priceCards pricinghttp.PriceCardCatalogueReader,
	referenceSeries pricinghttp.ReferenceSeriesCatalogueReader,
	networkCatalog networkhttp.OperationsCatalogReader,
	complianceRules customshttp.RuleCatalogueReader,
	serviceProducts commercialhttp.ServiceProductCatalogueReader,
	commercialPolicies commercialhttp.CommercialPolicyCatalogueReader,
) []httpapi.BusinessEndpoint {
	return []httpapi.BusinessEndpoint{
		{Pattern: "/shipment-requests", Handler: shipmenthttp.NewSubmitShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, submission)},
		{Pattern: "/shipment-requests/withdrawals", Handler: shipmenthttp.NewWithdrawShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, withdrawal)},
		{Pattern: "/shipment-requests/parcel-cancellations", Handler: shipmenthttp.NewCancelParcelEndpoint(shipmenthttp.UnconfiguredIntake{}, cancellation)},
		{Pattern: "/shipment-request-views", Handler: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(shipmenthttp.UnconfiguredIntake{}, requestViews)},
		{Pattern: "/node-operations/receptions", Handler: nodeopshttp.NewReceiveDeliveredUnitEndpoint(nodeopshttp.UnconfiguredIntake{}, reception)},
		{Pattern: "/transport-fulfillment/deliveries", Handler: tfhttp.NewRegisterEffectiveDeliveryEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/transport-fulfillment/delivery-proof-corrections", Handler: tfhttp.NewCorrectDeliveryProofEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/customer-tracking-view", Handler: visibilityhttp.NewQueryCustomerTrackingViewEndpoint(visibilityhttp.UnconfiguredIntake{}, trackingViews)},
		{Pattern: "/tracking-projections", Handler: visibilityhttp.NewQueryTrackingProjectionsEndpoint(visibilityhttp.UnconfiguredIntake{}, projectionViews)},
		{Pattern: "/claims", Handler: visibilityhttp.NewReceiveClaimEndpoint(visibilityhttp.UnconfiguredIntake{}, claims)},
		{Pattern: "/customs/external-results", Handler: customshttp.NewReceiveExternalResultEndpoint(customshttp.UnconfiguredIntake{}, results)},
		{Pattern: "/pricing-price-cards", Handler: pricinghttp.NewQueryPriceCardsEndpoint(pricinghttp.UnconfiguredIntake{}, priceCards)},
		{Pattern: "/pricing-reference-series", Handler: pricinghttp.NewQueryReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, referenceSeries)},
		{Pattern: "/network-catalog", Handler: networkhttp.NewQueryNetworkCatalogEndpoint(networkhttp.UnconfiguredIntake{}, networkCatalog)},
		{Pattern: "/customs-compliance-rules", Handler: customshttp.NewQueryComplianceRulesEndpoint(customshttp.UnconfiguredIntake{}, complianceRules)},
		{Pattern: "/commercial-service-products", Handler: commercialhttp.NewQueryServiceProductsEndpoint(commercialhttp.UnconfiguredIntake{}, serviceProducts)},
		{Pattern: "/commercial-policies", Handler: commercialhttp.NewQueryCommercialPoliciesEndpoint(commercialhttp.UnconfiguredIntake{}, commercialPolicies)},
	}
}
