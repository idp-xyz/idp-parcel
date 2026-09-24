package nodeopshttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：真实渠道归操作者渠道
// （查阅面 ADR-0100、作业事实 ADR-0149），真 Intake 未就位，装配点上还没有一行真渠道 Intake（ADR-0055）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置渠道参数，改请求或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让离线设备把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("node operations http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信
// 任何自报身份、不构造命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是 ReceptionIntake 注释所禁的「开发用」采信实现——那条红线禁的是从请求内容铸造
// 来源信封；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册并如实答未配置不是
// 默认实现，恰恰是它想保护的东西」。这里的空登记册就是装配点本身。ADR-0023 要求的
// 设备事实身份与时间同样无从谈起：连命令都不构造，也就没有任何一样东西被代铸。真渠道
// Intake 就位时在装配点替换，本类型随之退场，路由层与处理器不动（ADR-0055）。
type UnconfiguredIntake struct{}

var _ ReceptionIntake = UnconfiguredIntake{}
var _ CatalogueQueryIntake = UnconfiguredIntake{}

// IntakeReception 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeReception(context.Context, *http.Request) (application.ReceiveDeliveredUnitCommand, error) {
	return application.ReceiveDeliveredUnitCommand{}, ErrAccessChannelNotConfigured
}

// IntakeCatalogueQuery 同 IntakeReception：不读请求，只答未配置。命令面与查阅面
// 同堵——未配置是渠道这一层的状态，不是某个端点的状态，只堵命令就是给查阅留了条
// 无渠道也能进的路（判据同 tfhttp UnconfiguredIntake）。未登记前不铸造任何作用域
// （ADR-0077 Decision 三）。隔离读准入启用时由装配点换成
// IsolatedOperationsReadIntake，本类型在查阅面随之退场，命令面不受影响（ADR-0078）。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}
