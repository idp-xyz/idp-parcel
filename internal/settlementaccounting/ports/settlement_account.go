package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SettlementAccountRegistrationEffect 是登记一笔结算账户的三种落点。重放是同一标识、同一内容再交一次；
// 冲突是同一标识不同内容，或同一组固定属性挂到另一个标识上。两种都不改已落的行。
type SettlementAccountRegistrationEffect uint8

const (
	SettlementAccountRegistered SettlementAccountRegistrationEffect = iota + 1
	SettlementAccountReplay
	SettlementAccountConflict
)

// SettlementAccountView 是登记册的只读口。按固定属性交回账户标识；册上没有这一组就答 false，不代拟账户。
// 与写口分名：接受前控制的目录只该问账户，不该拿到 Save。
type SettlementAccountView interface {
	Find(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SettlementAccountKey,
	) (domain.SettlementAccountID, bool, error)
}

// SettlementAccountRegister 是结算账户登记册的写口。
type SettlementAccountRegister interface {
	Save(
		ctx context.Context,
		tenant domain.TenantID,
		account domain.SettlementAccount,
		at time.Time,
	) (SettlementAccountRegistrationEffect, error)
}

// SettlementAccountRegisteredIntent 是一笔结算账户登记落库后要通知下游的最小事实：哪个租户的哪一户。
//
// 下游是 parcel-shipment 停在`等待运营登记`的接受前财务控制——作用域那一停（`CONTROL_SCOPE_NOT_CONFIGURED`）
// 等的正是这一行。信封只说「这个租户的账户册上有新一户落库了，去重驱」，账户的法人、相对方、币种与结算
// 政策都不进载荷：重驱时作用域照旧向本上下文的账户册读口问，信封里再抄一份就成了第二处权威。
// RegisteredAt 是登记落库时刻，入队时刻由适配器的时钟另记。
type SettlementAccountRegisteredIntent struct {
	Tenant       domain.TenantID
	Account      domain.SettlementAccountID
	RegisteredAt time.Time
}

// ErrSettlementAccountHandoffRejected 表示交接口铸出的信封被框架的**确定性**校验拒收：账户标识长度归实例
// 半边、构造门只查非空，超长时主题过不了上限，同一份输入重投永远同一个结果。理由与形同
// ErrFundsFactHandoffRejected：编排整笔不作答、让事务壳回滚，登记口据此归用法格——归未决会让人照
// 「重跑」重跑到底，而账户行与信封一样都没落。
var ErrSettlementAccountHandoffRejected = errors.New("settlement accounting: settlement account registered envelope rejected by envelope validation")

// SettlementAccountRegisteredHandoff 把「结算账户已登记」写入 Outbox，与登记行同一事务（ADR-0094 决定四：
// `等待运营登记`那一格必须与它的续办触发同笔落地）。入队失败整项出错让事务回滚：行落了而信封没出，停在
// 作用域那一格的委托就再也没有投递来推它。信封被确定性拒收时交回的错误 errors.Is
// ErrSettlementAccountHandoffRejected；别的失败都是依赖不可用。
type SettlementAccountRegisteredHandoff interface {
	HandOffSettlementAccountRegistered(ctx context.Context, intent SettlementAccountRegisteredIntent) error
}
