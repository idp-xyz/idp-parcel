package customshttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：真实监管回执通道的
// 认证方式属 `PAR-INT-03` 待提供，装配点上还没有一行真通道 Intake（ADR-0055）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置通道参数，改报文或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让通道侧把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("customs compliance http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每份报文不读内容、不采信任何
// 自报身份、不构造命令，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是 ResultIntake 注释所禁的「开发用」采信实现——那条红线禁的是采信报文自称的租户
// 号（穿透 ADR-0003 的隔离边界）；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册
// 并如实答未配置不是默认实现，恰恰是它想保护的东西」。这里的空登记册就是装配点本身。
// ADR-0023 要求从报文体收的来源标识与发生时间同样无从谈起：连命令都不构造，也就没有
// 任何一样外部事实被服务端代铸。真通道 Intake 就位时在装配点替换，本类型随之退场，
// 路由层与处理器不动（ADR-0055）。
type UnconfiguredIntake struct{}

var _ ResultIntake = UnconfiguredIntake{}

// IntakeResult 不读报文。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeResult(context.Context, *http.Request) (application.ReceiveExternalResultCommand, error) {
	return application.ReceiveExternalResultCommand{}, ErrAccessChannelNotConfigured
}
