package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// OperatorRegistrationKind 是「参数已登记」信封里的登记种类，封闭集。
//
// 它答的是 parcel-shipment 那边`等待运营登记`第四格（ADR-0094 决定二）**等的是哪一类登记动作**，
// 不是登记了什么正文：正文是实例半边，留在本上下文的登记册上，信封只说「这一类参数在这个租户
// 里有新一版落库了，去重驱」。消费门不按种类分派（ADR-0094 决定四原话「不在消费门认原因名字」），
// 种类进信封是为了认领 ID 与事后追溯，两类登记各自认领、互不吞没。
type OperatorRegistrationKind uint8

const (
	OperatorRegistrationKindInvalid OperatorRegistrationKind = iota
	// AsOfPolicyRegistered：时点策略声明（`PAR-COM-14` 的时点半边）随规则包版本发布落库。对应
	// parcel-shipment 的 ReachabilityAsOfNotConfigured / FinancialControlAsOfNotConfigured。
	AsOfPolicyRegistered
	// AuthorityGrantRegistered：授权规则（`PAR-COM-14` 的授权半边，AuthorityGrantStore.SaveGrant）
	// 落库。对应 parcel-shipment 的 RejectionAuthorityRulesNotConfigured / WithdrawalAuthorityRulesNotConfigured /
	// SourceDataAmendmentAuthorityRulesNotConfigured。**今天没有任何生产编排发这一格**：SaveGrant 只有
	// 测试调，AuthorizedAction 里也还没有撤回与来源修订这两种动作；格留在封闭集里是为了让消费侧的
	// 契约一次写全，不是为了现在就有人发。
	AuthorityGrantRegistered
)

// String 交回信封载荷里的原词；集合外交回空串，交接适配器凭它把零值拦在入队之前。
func (kind OperatorRegistrationKind) String() string {
	switch kind {
	case AsOfPolicyRegistered:
		return "AS_OF_POLICY"
	case AuthorityGrantRegistered:
		return "AUTHORITY_GRANT"
	default:
		return ""
	}
}

// OperatorRegistrationCompletedIntent 是一次登记动作落库后要通知下游的最小事实。
//
// Registration 是承载这次登记的那一版商业对象（租户 / 类别 / 对象 / 版本号）：信封 ID 按
// ADR-0043 由它加种类认领，重放同一版重发同一封；它**不进载荷**——载荷只带租户与种类，下游按
// 租户重驱，不该也不需要知道是哪一版规则包。RegisteredAt 是领域发生时刻（发布时刻），入队时刻
// 由适配器的时钟另记，两者分开与「委托已提交」同一条纪律。
type OperatorRegistrationCompletedIntent struct {
	Kind         OperatorRegistrationKind
	Registration domain.CommercialVersion
	RegisteredAt time.Time
}

// OperatorRegistrationCompletedHandoff 把「参数已登记」写入 Outbox，与登记动作同一事务
// （ADR-0094 决定四：第四格必须与它的续办触发同笔落地）。
//
// 唯一实现是 adapters/postgres 的 Outbox 适配器；发布编排在声明落库之后、同一事务内调它。
// 入队失败整项出错让事务回滚：一份登记落了库而信封没入队，停在`等待运营登记`的委托就再也没有
// 投递来推它——那正是 ADR-0094 决定四说的「更安静的永久停滞」。
type OperatorRegistrationCompletedHandoff interface {
	HandOffOperatorRegistrationCompleted(ctx context.Context, intent OperatorRegistrationCompletedIntent) error
}
