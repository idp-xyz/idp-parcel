package accessidentity

import (
	"context"
	"errors"
	"strings"
)

// ErrIncompleteRegistration 表示一行登记缺件，建不成。
var ErrIncompleteRegistration = errors.New("access identity: channel registration is incomplete")

// ChannelRegistration 是登记册里的一行：这个渠道代表谁、凭据的受控引用在哪、以及
// 这个渠道的来源请求键怎么推导。
//
// 前三项是信封身份的**唯一**出处（ADR-0003）。它们不带 setter 也不导出字段：铸造之外
// 的任何地方都改不动一行登记所声明的租户，于是「报文里写别人的租户」在类型层就走不通，
// 不必靠每个 Intake 实现自己记得别读请求体。
//
// 本类型不是那张表的行结构。表结构等 PAR-INT-01 证据（ADR-0072 二），这里只是铸造这一
// 步需要的最小入参形状；立册时该长几列由那笔工作定，不由本类型倒推。
type ChannelRegistration struct {
	tenantID          string
	customerAccountID string
	source            string
	credential        CredentialReference
	derivation        RequestKeyDerivation
}

// NewChannelRegistration 要求推导口非空，是有意的：**没有推导口的登记行不许存在。**
//
// 若允许它缺席，铸造时就会多出「行在册但判重口径没配」这第三态，而它与「渠道未配置」
// 在答复上分不开——正是本包要治的折叠。要求同笔给全之后，铸造侧只剩未配置与凭据不符
// 两答，那两答的恢复动作确实不同。
func NewChannelRegistration(
	tenantID string,
	customerAccountID string,
	source string,
	credential CredentialReference,
	derivation RequestKeyDerivation,
) (ChannelRegistration, error) {
	tenant := strings.TrimSpace(tenantID)
	account := strings.TrimSpace(customerAccountID)
	origin := strings.TrimSpace(source)
	if tenant == "" || account == "" || origin == "" || credential.value == "" || derivation == nil {
		return ChannelRegistration{}, ErrIncompleteRegistration
	}
	return ChannelRegistration{
		tenantID:          tenant,
		customerAccountID: account,
		source:            origin,
		credential:        credential,
		derivation:        derivation,
	}, nil
}

func (registration ChannelRegistration) TenantID() string { return registration.tenantID }
func (registration ChannelRegistration) CustomerAccountID() string {
	return registration.customerAccountID
}
func (registration ChannelRegistration) Source() string { return registration.source }

func (registration ChannelRegistration) CredentialReference() CredentialReference {
	return registration.credential
}

// ChannelRegistry 是登记册的装载口。
//
// 册里没有这一行时答 found=false 而**不返回 error**：空册与读不动是两件事，恢复动作
// 一个是去配渠道、一个是去救依赖（ADR-0052 的分界句同款成立）。用 error 顶掉未配置，
// 运维会去救一个没坏的依赖。
//
// 本仓此刻没有它的生产实现——按 ADR-0072 二，表与迁移等 PAR-INT-01 证据。
type ChannelRegistry interface {
	FindChannel(ctx context.Context, key ChannelKey) (ChannelRegistration, bool, error)
}

// RequestKeyDerivation 决定**哪个渠道字段铸成来源请求键**。
//
// 这一格是 PAR-INT-01 最低证据里「重试和冲突边界」那一件定的，客户尚未回复：同一委托
// 重复提交按业务标识还是渠道请求键判重、判重命中时渠道期待什么答复，都还没有答案。
// 来源请求键就是幂等键本身，所以这里**不带任何生产实现，也不取默认值**——按 AGENTS.md
// 红线，未确认参数保持显式未决，不写死为生产默认。
//
// 提交与撤回各有一个方法而不是共用一个带动作参数的方法：同一个客户就同一份委托先提交
// 后撤回是两次来源请求，共用一个键会让撤回被判成原提交的重放，此后再也分不出「这单
// 重发了」与「这单要撤」。两个方法让这件事在实现方那一侧也没法含糊过去。
type RequestKeyDerivation interface {
	DeriveSubmissionRequestKey(ctx context.Context, request ChannelRequest) (string, error)
	DeriveWithdrawalRequestKey(ctx context.Context, request ChannelRequest) (string, error)
}

// ChannelRequest 是渠道原始请求交给推导口的只读面。
//
// 刻意不是 *http.Request：ADR-0072 一说的渠道有 API、标准文件与门户三形，把某一形的
// 传输类型钉进接口，等于替另两形拟了形状——正是 ADR-0055 那条被维持的否决所禁的事。
type ChannelRequest struct {
	values map[string]string
}

func NewChannelRequest(values map[string]string) ChannelRequest {
	copied := make(map[string]string, len(values))
	for name, value := range values {
		copied[name] = value
	}
	return ChannelRequest{values: copied}
}

func (request ChannelRequest) Value(name string) (string, bool) {
	value, ok := request.values[name]
	return value, ok
}
