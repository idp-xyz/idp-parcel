package main

import (
	"context"
	"errors"

	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
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
// 连一个零值结果都不交回。真渠道 Intake 就位那笔工作在装配点把它们换成真编排，与替换
// Intake 同一处、同一行。
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

// unwiredTrackingViews 顶的是读口不是编排，但填法同理：读不回按 ADR-0022 是「没形成
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
