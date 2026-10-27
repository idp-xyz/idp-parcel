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

// refusalHandledResumeConsumerName 是「释放拒收已处理」续办门在 inbox 键上的稳定名。各扇门转交同一条链，但各收各的信；共名会让名册读起来像同一扇
// （理由同 manualReviewResumeConsumerName）。
const refusalHandledResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-control-release-refusal-handled"

// ControlReleaseRefusalHandledEventType 是本门认的事件类型（ADR-0293 决定四的续办触发）。发布侧是本上下文的释放拒收处理 Outbox 交接适配器，两边各写各的
// 字面，理由与「复核已完成」那对相同；两串漂开撞 dispatch.no_subscriber，载荷字段漂开由 cmd/parcel-dispatch 的真链往返用例揪红。
//
// 新立一类而不复用既有的某一类：既有几类各说各的事（提交、复核完成、参数已登记、新提交版本已形成），拿其中任何一类顶替都是让信封的类型名说假话，
// 而类型名正是运维读 outbox 与死信时唯一的线索；它们又各自铸信封的口、用各自的信封 ID，借用只会撞上「同一版本至多一封」的去重。门里的译码与折法则
// 是共用的，新增的只是一个类型与一个 inbox 名。
const ControlReleaseRefusalHandledEventType eventing.EventType = "parcel-shipment.shipment-request.control-release-refusal-handled"

// ControlReleaseRefusalHandledConsumer 把「释放拒收已处理」信封再驱一遍接受判断链（ADR-0293 决定四）。
//
// 它与提交门、复核续办门、受控补充续办门转交**同一个**编排、共用同一份译码与同一个折法。信封携带的是委托**此刻的当前版本**：接受链开头放被取代各版
// 遇拒收响亮失败（ADR-0282 决定三）落 publish_failed 之后，人把被拒收的那一笔处理完，链拿当前版本重驱，开头那一步凭留痕认出这一笔已处理并放行，新版本能判。
// 重驱幂等：当前版本已决定的，链的入口守卫与既有决定路径答`已决定`，不再占下任何东西；信封里的版本不是委托当前版本的（处理之后又换了代），链答
// `版本已换代`按处理完毕入账，新版本由它自己的「新提交版本已形成」信封驱动。
type ControlReleaseRefusalHandledConsumer struct {
	gate *inboxconsume.Gate[psapplication.AdvanceAcceptanceChainCommand]
}

func NewControlReleaseRefusalHandledConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	advancer AcceptanceChainAdvancer,
) (*ControlReleaseRefusalHandledConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if advancer == nil {
		return nil, fmt.Errorf("parcel shipment inbox: acceptance chain advancer is nil")
	}
	consumer := &ControlReleaseRefusalHandledConsumer{}
	gate, err := inboxconsume.New(inboxconsume.Spec[psapplication.AdvanceAcceptanceChainCommand]{
		Transactor:     transactor,
		Store:          store,
		Name:           refusalHandledResumeConsumerName,
		EventType:      ControlReleaseRefusalHandledEventType,
		Decode:         decodeAcceptanceChainPayload,
		Handle:         advanceAcceptanceChainThrough(advancer),
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "advance acceptance chain on control release refusal handled",
	})
	if err != nil {
		return nil, err
	}
	consumer.gate = gate
	return consumer, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与同包其余消费者共用同一扇门。
func (consumer *ControlReleaseRefusalHandledConsumer) Consume(
	ctx context.Context,
	envelope eventing.Envelope,
) error {
	return consumer.gate.Consume(ctx, envelope)
}
