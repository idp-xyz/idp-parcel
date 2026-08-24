package main

import (
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
)

// assembleBusinessEndpoints 是业务端点的装配点（组合根）。五个上下文的 adapters/http
// 里的接入面处理器全部挂在这里：PS 提交、撤回与委托查阅、NO 收寄登记、TF 交付登记与
// POD 更正、VE 视图查询与索赔受理、CC 外部结果接收。
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
// 各端点的第二参（应用编排或读口）与 Intake 是两笔独立的接线：提交编排已按审计票 13
// 接真，撤回编排随 UI 阶段 B 后端序列接真，NO 收寄编排、TF 交付双端点与 VE 索赔受理
// 按接线票 `.scratch/parcel-api-remaining-endpoint-wiring` 的票 01、02、04 接真（均经
// 真库，由 main 构造后入参交入），委托查阅与 VE 客户追踪视图两个读口接真库读适配器；
// 余下一格（CC 外部结果）仍以 unwired* 占位，它的接线自成一笔。占位与接真的分辨见
// unwired_orchestration.go 的文件注释。
//
// 清单是九项（PS 三、NO 一、TF 二、VE 二、CC 一）。ADR-0055 与开发主线曾把它称作
// 「七个」，那是把 TF 双端点计作一项的算术口径错，后按逐项枚举定为八项；第九项是
// 委托查阅（GET /shipment-request-views，UI 阶段 B 的读切片）。此处按逐项枚举装配，
// 少装一个就是把一个端点折回 404，那正是该记录要治的病。
//
// requestViews 与 trackingViews 是两个查阅端点的读口：读面不是编排（查阅不触发判断、
// 派生或披露），生产装配交入各自的真库读适配器；未配置 Intake 仍拒在它们之前，接入
// 渠道就位前一次也不会被调到。
func assembleBusinessEndpoints(
	submission shipmenthttp.SubmissionHandler,
	withdrawal shipmenthttp.WithdrawalHandler,
	requestViews shipmenthttp.ShipmentRequestViewsReader,
	reception nodeopshttp.ReceptionHandler,
	delivery tfhttp.DeliveryHandler,
	trackingViews visibilityhttp.TrackingViewReader,
	claims visibilityhttp.ClaimReceiver,
) []httpapi.BusinessEndpoint {
	return []httpapi.BusinessEndpoint{
		{Pattern: "/shipment-requests", Handler: shipmenthttp.NewSubmitShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, submission)},
		{Pattern: "/shipment-requests/withdrawals", Handler: shipmenthttp.NewWithdrawShipmentRequestEndpoint(shipmenthttp.UnconfiguredIntake{}, withdrawal)},
		{Pattern: "/shipment-request-views", Handler: shipmenthttp.NewQueryShipmentRequestViewsEndpoint(shipmenthttp.UnconfiguredIntake{}, requestViews)},
		{Pattern: "/node-operations/receptions", Handler: nodeopshttp.NewReceiveDeliveredUnitEndpoint(nodeopshttp.UnconfiguredIntake{}, reception)},
		{Pattern: "/transport-fulfillment/deliveries", Handler: tfhttp.NewRegisterEffectiveDeliveryEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/transport-fulfillment/delivery-proof-corrections", Handler: tfhttp.NewCorrectDeliveryProofEndpoint(tfhttp.UnconfiguredIntake{}, delivery)},
		{Pattern: "/customer-tracking-view", Handler: visibilityhttp.NewQueryCustomerTrackingViewEndpoint(visibilityhttp.UnconfiguredIntake{}, trackingViews)},
		{Pattern: "/claims", Handler: visibilityhttp.NewReceiveClaimEndpoint(visibilityhttp.UnconfiguredIntake{}, claims)},
		{Pattern: "/customs/external-results", Handler: customshttp.NewReceiveExternalResultEndpoint(customshttp.UnconfiguredIntake{}, unwiredResults{})},
	}
}
