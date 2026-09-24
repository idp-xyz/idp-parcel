package governancehttp

import (
	"context"
	"errors"
	"net/http"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道：运营接入认证属
// 操作者渠道（ADR-0100），其真 Intake 未就位，装配点上还没有一行真渠道 Intake
// （ADR-0055、ADR-0077 Decision 三）。
//
// 恢复动作判据同 ADR-0029：这一格要接入方去提供并配置渠道参数，改请求或重试都不会
// 好。本包据以回 403 + ACCESS_CHANNEL_NOT_CONFIGURED；折进 404 会与「产品没有这个
// 能力」不可分辨，折进 5xx 会让客户端把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("pilot governance http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态，不命名参数（ADR-0055）。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复：对每个请求不读业务内容、不采信
// 任何自报身份、不构造查询，一律交回 ErrAccessChannelNotConfigured。
//
// 它不是被禁的「开发用」采信实现——那条红线禁的是采信自报身份作授权（ADR-0022→
// ADR-0003 链）；本类型恰是其反面。真渠道就位时在装配点替换，本类型随之退场。
type UnconfiguredIntake struct{}

var _ RegistryQueryIntake = UnconfiguredIntake{}

// IntakeRegistryQuery 不读请求。参数刻意匿名：连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeRegistryQuery(context.Context, *http.Request) (RegistryQuery, error) {
	return RegistryQuery{}, ErrAccessChannelNotConfigured
}
