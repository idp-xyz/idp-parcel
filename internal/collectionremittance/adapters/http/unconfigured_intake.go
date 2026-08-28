package collectionhttp

import (
	"context"
	"errors"
	"net/http"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道:运营查阅接入面的
// 认证方式属 `PAR-INT-01` 待提供,装配点上还没有一行真通道 Intake(ADR-0055)。哨兵
// 只此一个而不随端点分设:未配置是渠道这一层的状态,按端点分设哨兵会让装配点看起来
// 能只配一半(判据同 customshttp)。
//
// 它与 ErrMalformedRequest、依赖故障分成三格,判据同 ADR-0029——恢复动作不同:这一格
// 要接入方去提供并配置通道参数,改报文或重试都不会好。本包据以回 403 +
// ACCESS_CHANNEL_NOT_CONFIGURED;折进 404 会与「产品没有这个能力」不可分辨,折进
// INTAKE_FAILED(5xx)会让通道侧把一件人不来配就永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("collection remittance http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态,不命名参数(ADR-0055):这里等的是哪个
// 登记册行由参数登记册说,错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复:对每份请求不读内容、不采信任何
// 自报身份、不铸造任何作用域,一律交回 ErrAccessChannelNotConfigured。
//
// 它不是被禁的「开发用」采信实现——那条红线禁的是采信报文自称的租户号(穿透 ADR-0003
// 的隔离边界);本类型恰是其反面,分界同 ADR-0052:「读一个空登记册并如实答未配置
// 不是默认实现,恰恰是它想保护的东西」,这里的空登记册就是装配点本身。本包只有查阅
// 端点、没有命令面,故本类型也只实现 CatalogueQueryIntake。真通道 Intake 就位时在
// 装配点替换,本类型随之退场,路由层与处理器不动(ADR-0055)。
type UnconfiguredIntake struct{}

var _ CatalogueQueryIntake = UnconfiguredIntake{}

// IntakeCatalogueQuery 不读请求(参数刻意匿名:连签名都不给「读一眼再决定」留位置),
// 只答未配置。未登记前不铸造任何作用域(ADR-0077 Decision 三)。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}
