package psinbox

import (
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// settlementAccountResumeConsumerName 是「结算账户已登记」续办门在 inbox 键上的稳定名，与 party-commercial 那一份
// 各占一个名（理由见 OperatorRegistrationCompletedConsumer 的类型注释）。
const settlementAccountResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-settlement-account-registration"

// SettlementAccountRegisteredEventType 是本门认的事件类型：settlement-accounting 登记一笔结算账户时与登记行同一
// 事务交出（票 routing-first-cut/19）。它解开的是接受前财务控制停在作用域那一格的等待（`CONTROL_SCOPE_NOT_CONFIGURED`，
// 端口的`未配置`格）。发布侧是 settlement-accounting 的 Outbox 交接适配器，两边各写各的字面；两串漂开撞
// dispatch.no_subscriber，载荷漂开由 cmd/parcel-dispatch 那条用真 SA 适配器铸封的往返用例揪红。
const SettlementAccountRegisteredEventType eventing.EventType = "settlement-accounting.settlement-account.registered"

// NewSettlementAccountRegisteredConsumer 装配收 settlement-accounting「结算账户已登记」的那一份登记续办门。重驱
// 纪律与 party-commercial 那一份完全相同：按租户取回`等待运营登记`的队列逐份再驱一拍，不认是哪一户账户——链
// 重跑到作用域那一步自会向账户册问，这一户对不上这份委托就再停一次。
func NewSettlementAccountRegisteredConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	queue ports.OperatorRegistrationQueue,
	advancer AcceptanceChainAdvancer,
	pageSize int,
) (*OperatorRegistrationCompletedConsumer, error) {
	return newRegistrationResumeConsumer(
		settlementAccountResumeConsumerName,
		SettlementAccountRegisteredEventType,
		"advance acceptance chain on settlement account registration",
		transactor, store, queue, advancer, pageSize,
	)
}
