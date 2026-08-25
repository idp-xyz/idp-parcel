package pricinghttp

import (
	"context"
	"errors"
	"net/http"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道:运营接入认证属
// 接入渠道实例半边、`PAR-INT-01` 未登记,装配点上还没有一行真渠道 Intake
// (ADR-0055、ADR-0077 Decision 三)。
//
// 恢复动作判据同 ADR-0029:这一格要接入方去提供并配置渠道参数,改请求或重试都不会
// 好。哨兵只此一个而不随端点分设:未配置是渠道这一层的状态,按端点分设哨兵会让装配
// 点看起来能只配一半。本包据以回 403 + ACCESS_CHANNEL_NOT_CONFIGURED;折进 404 会与
// 「产品没有这个能力」不可分辨,折进 INTAKE_FAILED(5xx)会让客户端把一件人不来配就
// 永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("parcel pricing http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态,不命名参数(ADR-0055):这里等的是哪个
// 登记册行由参数登记册说,错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复:对每个请求不读业务内容、不采信
// 任何自报身份、不构造查询,一律交回 ErrAccessChannelNotConfigured。
//
// 它不是被禁的「开发用」采信实现——那条红线禁的是采信自报租户(穿透 ADR-0003 的
// 隔离);本类型恰是其反面,分界同 ADR-0052:「读一个空登记册并如实答未配置不是默认
// 实现,恰恰是它想保护的东西」。这里的空登记册就是装配点本身。真渠道就位时在装配点
// 替换,本类型随之退场。
type UnconfiguredIntake struct{}

var _ PricingCatalogueIntake = UnconfiguredIntake{}

// IntakeCatalogueQuery 不读请求。参数刻意匿名:连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}
