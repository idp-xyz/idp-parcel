package shipmenthttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：真实渠道的认证方式
// 属 `PAR-INT-01` 待提供，装配点上还没有一行真渠道 Intake（ADR-0055）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置渠道参数，改请求或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让离线客户端把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("parcel shipment http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信
// 任何自报身份、不构造命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是各 Intake 注释所禁的「开发用」采信实现——那条红线禁的是从请求内容铸造来源
// 信封；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册并如实答未配置不是默认
// 实现，恰恰是它想保护的东西」。这里的空登记册就是装配点本身。真渠道 Intake 就位时
// 在装配点逐端点替换，本类型随之退场，路由层与处理器不动（ADR-0055）。
type UnconfiguredIntake struct{}

var (
	_ SubmissionIntake             = UnconfiguredIntake{}
	_ WithdrawalIntake             = UnconfiguredIntake{}
	_ ShipmentRequestViewsIntake   = UnconfiguredIntake{}
	_ CancellationIntake           = UnconfiguredIntake{}
	_ ManualReviewCompletionIntake = UnconfiguredIntake{}
	_ ActiveRejectionIntake        = UnconfiguredIntake{}
)

// IntakeSubmission 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeSubmission(context.Context, *http.Request) (application.SubmitShipmentRequestCommand, error) {
	return application.SubmitShipmentRequestCommand{}, ErrAccessChannelNotConfigured
}

// IntakeWithdrawal 同 IntakeSubmission：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeWithdrawal(context.Context, *http.Request) (application.WithdrawShipmentRequestCommand, error) {
	return application.WithdrawShipmentRequestCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCancellation 同上。
func (UnconfiguredIntake) IntakeCancellation(context.Context, *http.Request) (application.CancelParcelCommand, error) {
	return application.CancelParcelCommand{}, ErrAccessChannelNotConfigured
}

// IntakeManualReviewCompletion 同上：复核人与授权引用整组来自认证结果，采信自报的
// 复核人等于让任何调用方替任何角色签复核。
func (UnconfiguredIntake) IntakeManualReviewCompletion(context.Context, *http.Request) (application.CompleteManualReviewCommand, error) {
	return application.CompleteManualReviewCommand{}, ErrAccessChannelNotConfigured
}

// IntakeActiveRejection 同上。
func (UnconfiguredIntake) IntakeActiveRejection(context.Context, *http.Request) (application.RejectShipmentRequestCommand, error) {
	return application.RejectShipmentRequestCommand{}, ErrAccessChannelNotConfigured
}

// IntakeListQuery 同上：查阅的作用域整组来自认证与授权结果，渠道未配置就无从铸造。
func (UnconfiguredIntake) IntakeListQuery(context.Context, *http.Request) (ShipmentRequestViewsQuery, error) {
	return ShipmentRequestViewsQuery{}, ErrAccessChannelNotConfigured
}

// IntakeDetailQuery 同上。
func (UnconfiguredIntake) IntakeDetailQuery(context.Context, *http.Request) (ShipmentRequestViewQuery, error) {
	return ShipmentRequestViewQuery{}, ErrAccessChannelNotConfigured
}
