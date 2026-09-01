package psinbox

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// manualReviewResumeConsumerName 是续办门在 inbox 键上的稳定名。与「委托已提交」那扇门
// 各占一个名：inbox 键由（消费者名 + 来源 + 事件 ID）认领，两扇门本就收不同类型的信，
// 但共名会让名册读起来像同一扇。
const manualReviewResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-review-completion"

// ManualReviewCompletedEventType 是续办门认的事件类型（ADR-0086 Decision 二）。发布侧是
// 本上下文的复核完成 Outbox 交接适配器，两边各写各的字符串，理由与「委托已提交」那对
// 相同；两串漂开撞 dispatch.no_subscriber，载荷字段漂开由 cmd/parcel-api 的铸封译码用例
// 守着。
const ManualReviewCompletedEventType eventing.EventType = "parcel-shipment.shipment-request.manual-review-completed"

// ManualReviewCompletedConsumer 把「复核已完成」信封再驱一遍接受判断链（ADR-0086）。
//
// 它与 ShipmentRequestSubmittedConsumer 转交**同一个**编排、共用同一份译码与同一个折法：
// 重跑的链在可达性与财务控制两步读回已记录判断即过，形成决定一步读到已完成的复核即成
// 决定。停在其它未决原因时照旧回滚重投（重跑等的依赖会自己回来）；再次停在`等待人工复核`
// ——构造上只剩新提交版本换代重开复核这一种可能——按暂停入账，与提交门同一格。
type ManualReviewCompletedConsumer struct {
	gate *inboxconsume.Gate[psapplication.AdvanceAcceptanceChainCommand]
}

func NewManualReviewCompletedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	advancer AcceptanceChainAdvancer,
) (*ManualReviewCompletedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if advancer == nil {
		return nil, fmt.Errorf("parcel shipment inbox: acceptance chain advancer is nil")
	}
	consumer := &ManualReviewCompletedConsumer{}
	gate, err := inboxconsume.New(inboxconsume.Spec[psapplication.AdvanceAcceptanceChainCommand]{
		Transactor:     transactor,
		Store:          store,
		Name:           manualReviewResumeConsumerName,
		EventType:      ManualReviewCompletedEventType,
		Decode:         decodeAcceptanceChainPayload,
		Handle:         advanceAcceptanceChainThrough(advancer),
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "advance acceptance chain on review completion",
	})
	if err != nil {
		return nil, err
	}
	consumer.gate = gate
	return consumer, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与同包其余消费者共用同一扇门。
func (consumer *ManualReviewCompletedConsumer) Consume(
	ctx context.Context,
	envelope eventing.Envelope,
) error {
	return consumer.gate.Consume(ctx, envelope)
}
