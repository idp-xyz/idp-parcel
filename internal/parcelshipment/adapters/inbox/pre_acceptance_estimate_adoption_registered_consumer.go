package psinbox

import (
	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// estimateAdoptionResumeConsumerName 是「受理前估价采用已登记」续办门在 inbox 键上的稳定名，与另两份登记续办门各占一个名
// （理由见 OperatorRegistrationCompletedConsumer 的类型注释）。
const estimateAdoptionResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-pre-acceptance-estimate-adoption"

// PreAcceptanceEstimateAdoptionRegisteredEventType 是本门认的事件类型：settlement-accounting 登记一行受理前估价采用时与采用行同一
// 事务交出（票 product-strategy-boundary/25，ADR-0179 决定五）。它要解开的是接受前财务控制停在金额那一格的等待。发布侧是
// settlement-accounting 的 Outbox 交接适配器，两边各写各的字面。
const PreAcceptanceEstimateAdoptionRegisteredEventType eventing.EventType = "settlement-accounting.pre-acceptance-estimate-adoption.registered"

// NewPreAcceptanceEstimateAdoptionRegisteredConsumer 装配收「受理前估价采用已登记」的那一份登记续办门。重驱纪律与另两份完全相同：
// 按租户取回`等待运营登记`的队列逐份再驱一拍，不认是哪一户——链重跑到控制那一步自会向采用册问。
func NewPreAcceptanceEstimateAdoptionRegisteredConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	queue ports.OperatorRegistrationQueue,
	advancer AcceptanceChainAdvancer,
	pageSize int,
) (*OperatorRegistrationCompletedConsumer, error) {
	return newRegistrationResumeConsumer(
		estimateAdoptionResumeConsumerName,
		PreAcceptanceEstimateAdoptionRegisteredEventType,
		"advance acceptance chain on pre-acceptance estimate adoption registration",
		transactor, store, queue, advancer, pageSize,
	)
}
