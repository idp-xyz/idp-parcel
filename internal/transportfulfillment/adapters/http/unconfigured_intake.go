package tfhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：真实渠道的认证方式
// 属 `PAR-INT-01` 待提供，装配点上还没有一行真渠道 Intake（ADR-0055）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置渠道参数，改请求或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让离线设备把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("transport fulfillment http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信
// 任何自报身份、不构造命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是 DeliveryIntake 注释所禁的「开发用」采信实现——那条红线禁的是从请求内容铸造
// 来源信封；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册并如实答未配置不是
// 默认实现，恰恰是它想保护的东西」。这里的空登记册就是装配点本身。ADR-0023 要求的
// POD 证据与更正时间同样无从谈起：连命令都不构造，也就没有任何一样东西被代铸。
//
// 首登与更正两个方法都堵住：未配置是渠道这一层的状态，不是某个端点的状态，只堵一个
// 就是给另一个留了条无渠道也能进的路。真渠道 Intake 就位时在装配点替换，本类型随之
// 退场，路由层与处理器不动（ADR-0055）。
type UnconfiguredIntake struct{}

var _ DeliveryIntake = UnconfiguredIntake{}
var _ CatalogueQueryIntake = UnconfiguredIntake{}

// IntakeRegistration 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeRegistration(context.Context, *http.Request) (application.RegisterEffectiveDeliveryCommand, error) {
	return application.RegisterEffectiveDeliveryCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCorrection 同 IntakeRegistration：不读请求，只答未配置。
func (UnconfiguredIntake) IntakeCorrection(context.Context, *http.Request) (application.CorrectDeliveryProofCommand, error) {
	return application.CorrectDeliveryProofCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCatalogueQuery 同上两个方法：不读请求，只答未配置。命令面与查阅面同堵——
// 本类型注释立的正是这条：未配置是渠道这一层的状态，不是某个端点的状态，只堵命令
// 就是给查阅留了条无渠道也能进的路。未登记前不铸造任何作用域（ADR-0077 Decision
// 三）。隔离读准入启用时由装配点换成 IsolatedOperationsReadIntake，本类型在查阅面
// 随之退场，命令面不受影响（ADR-0078）。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}
