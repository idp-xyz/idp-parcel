package settlementhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：运营查阅接入面的
// 认证归操作者渠道（ADR-0100），其真 Intake 未就位，装配点上还没有一行真通道 Intake（ADR-0055）。哨兵
// 只此一个而不随端点分设：未配置是渠道这一层的状态，按端点分设哨兵会让装配点看起来
// 能只配一半（判据同 customshttp）。
//
// 它与 ErrMalformedRequest、依赖故障分成三格，判据同 ADR-0029——恢复动作不同：这一格
// 要接入方去提供并配置通道参数，改报文或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个能力」不可分辨，折进
// INTAKE_FAILED（5xx）会让通道侧把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("settlement accounting http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）：这里等的是哪个
// 登记册行由参数登记册说，错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每份请求不读内容、不采信任何
// 自报身份、不铸造任何作用域，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是被禁的「开发用」采信实现——那条红线禁的是采信报文自称的租户号（穿透 ADR-0003
// 的隔离边界）；本类型恰是其反面，分界同 ADR-0052：「读一个空登记册并如实答未配置
// 不是默认实现，恰恰是它想保护的东西」，这里的空登记册就是装配点本身。查阅端点与
// 命令端点的未配置实现都在本类型上：写准入不另立形（ADR-0085 决定二），命令端点在
// 装配表里同挂字面量 UnconfiguredIntake{} 起步；隔离读放行 IsolatedOperationsReadIntake
// 刻意不实现命令 Intake，放行装不进命令端点由编译期决定（ADR-0078 隔离读准入不扩到
// 写行）。真通道 Intake 就位时在装配点替换，本类型随之退场，路由层与各处理器不动
// （ADR-0055）。
type UnconfiguredIntake struct{}

var _ CatalogueQueryIntake = UnconfiguredIntake{}

// IntakeCatalogueQuery 不读请求（参数刻意匿名：连签名都不给「读一眼再决定」留位置），
// 只答未配置。未登记前不铸造任何作用域（ADR-0077 Decision 三）。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}

// 外部资金事实采用与更正两口的未配置实现（票 sa-cc/31）：与查阅口同一分界——不读业务内容、
// 不采信自报身份、不构造命令。载荷里的 tenantId 是这一面最危险的自报身份，本类型连解析都不做。
var (
	_ ExternalFundsFactRegistrationIntake           = UnconfiguredIntake{}
	_ ExternalFundsFactCorrectionRegistrationIntake = UnconfiguredIntake{}
)

// IntakeExternalFundsFactRegistration 不读请求，判据同 IntakeCatalogueQuery。下一个同此。
func (UnconfiguredIntake) IntakeExternalFundsFactRegistration(
	context.Context,
	*http.Request,
) (application.AdoptFundsFactCommand, error) {
	return application.AdoptFundsFactCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeExternalFundsFactCorrectionRegistration(
	context.Context,
	*http.Request,
) (application.CorrectFundsFactCommand, error) {
	return application.CorrectFundsFactCommand{}, ErrAccessChannelNotConfigured
}
