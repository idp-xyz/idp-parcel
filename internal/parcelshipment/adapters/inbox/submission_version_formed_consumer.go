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

// supplementResumeConsumerName 是「新提交版本已形成」续办门在 inbox 键上的稳定名。各扇门转交
// 同一条链，但各收各的信；共名会让名册读起来像同一扇（理由同 manualReviewResumeConsumerName）。
const supplementResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-submission-version-formed"

// SubmissionVersionFormedEventType 是本门认的事件类型（ADR-0106 Decision 三的续办触发）。发布侧是
// 本上下文的受控补充 Outbox 交接适配器，两边各写各的字面，理由与「复核已完成」那对相同；两串漂开
// 撞 dispatch.no_subscriber，载荷字段漂开由 cmd/parcel-dispatch 的真链往返用例揪红。词取 ADR-0045
// 「新提交版本」。
const SubmissionVersionFormedEventType eventing.EventType = "parcel-shipment.shipment-request.submission-version-formed"

// SubmissionVersionFormedConsumer 把「新提交版本已形成」信封再驱一遍接受判断链（ADR-0106）。
//
// 它与提交门、复核续办门转交**同一个**编排、共用同一份译码与同一个折法。信封携带的是**新**
// 提交版本：停在`等待受控补充`的那一版已随入账留在库里，链拿新版本的任务重跑，在受控补充那一步
// 不再停；停在其它未决原因时照旧按恢复动作折——依赖抖动回滚重投，再次停在续办方在进程之外的
// 等待态即再暂停入账一次。
//
// 门里不判「新版本是不是真的比停住的那版新」：那是编排读聚合时的事（基准过期由
// FormNewSubmissionVersionHandler 在形成版本那一步就拒了），在消费门里再判等于第二处持有版本
// 顺序的知识（ADR-0106 Alternatives 第三条）。
type SubmissionVersionFormedConsumer struct {
	gate *inboxconsume.Gate[psapplication.AdvanceAcceptanceChainCommand]
}

func NewSubmissionVersionFormedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	advancer AcceptanceChainAdvancer,
) (*SubmissionVersionFormedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if advancer == nil {
		return nil, fmt.Errorf("parcel shipment inbox: acceptance chain advancer is nil")
	}
	consumer := &SubmissionVersionFormedConsumer{}
	gate, err := inboxconsume.New(inboxconsume.Spec[psapplication.AdvanceAcceptanceChainCommand]{
		Transactor:     transactor,
		Store:          store,
		Name:           supplementResumeConsumerName,
		EventType:      SubmissionVersionFormedEventType,
		Decode:         decodeAcceptanceChainPayload,
		Handle:         advanceAcceptanceChainThrough(advancer),
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "advance acceptance chain on submission version formed",
	})
	if err != nil {
		return nil, err
	}
	consumer.gate = gate
	return consumer, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与同包其余消费者共用同一扇门。
func (consumer *SubmissionVersionFormedConsumer) Consume(
	ctx context.Context,
	envelope eventing.Envelope,
) error {
	return consumer.gate.Consume(ctx, envelope)
}
