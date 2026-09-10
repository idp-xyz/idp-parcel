package psinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// labelTransactionJudgmentConsumerName 是本消费者在 inbox 键上的稳定名。与交付 / 收寄 / 揽收各路
// 分开：Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。
// 改名等于换消费者。
const labelTransactionJudgmentConsumerName = "parcel-shipment/judge-label-final-from-transaction"

// LabelTransactionJudgmentDueEventType 是本消费者认的事件类型：面单交易写侧在定案与后续动作两拍
// `Save` 成功后交出的「这件包裹值得判一次终局」指针式信封（ADR-0134 决定一 / 四）。两拍共用一个类型，
// 一扇门一个处理方。消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
const LabelTransactionJudgmentDueEventType eventing.EventType = "parcel-shipment.label-transaction.judgment-due"

// LabelTransactionJudgmentDue 是译码后的指针：租户、触发的交易、要判的包裹。**不带交易本体，处理方也
// 不按它读回交易**——判断的输入是此刻该包裹的全部相关交易与登记册（CONTEXT「必须跨该包裹全部相关
// 交易及实际承运商收寄事实形成包裹级判断」），信封指的是一拍而不是一份要被读回的事实版本。
// revision 与 beat 不在这里：版本是事件 ID 的事（让两拍各自入队），哪一拍只作追溯、消费者不据它分支。
type LabelTransactionJudgmentDue struct {
	TenantID    string
	Transaction string
	Parcel      string
}

// LabelTransactionJudgmentDueHandler 是本消费者转交的处理方。真实装配接三路共用的处理方核
// （adapters/labelfinal）。
type LabelTransactionJudgmentDueHandler interface {
	HandleLabelTransactionJudgmentDue(ctx context.Context, due LabelTransactionJudgmentDue) error
}

// LabelTransactionJudgmentConsumer 把面单交易判断意图信封推进消费门。
type LabelTransactionJudgmentConsumer struct {
	gate *inboxconsume.Gate[LabelTransactionJudgmentDue]
}

func NewLabelTransactionJudgmentConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler LabelTransactionJudgmentDueHandler,
) (*LabelTransactionJudgmentConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel shipment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[LabelTransactionJudgmentDue]{
		Transactor:     transactor,
		Store:          store,
		Name:           labelTransactionJudgmentConsumerName,
		EventType:      LabelTransactionJudgmentDueEventType,
		Decode:         decodeLabelTransactionJudgmentDue,
		Handle:         handler.HandleLabelTransactionJudgmentDue,
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "handle label transaction judgment due",
	})
	if err != nil {
		return nil, err
	}
	return &LabelTransactionJudgmentConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余消费者共用同一扇门。
func (consumer *LabelTransactionJudgmentConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeLabelTransactionJudgmentDue 译载荷。三维缺一即毒丸——处理方按（租户 + 包裹）反查目标委托、
// 按交易追溯，缺了永远补不上，而重投同样内容不会长出字段来。revision 与 beat 故意不读。
func decodeLabelTransactionJudgmentDue(payload []byte) (LabelTransactionJudgmentDue, error) {
	var body struct {
		TenantID    string `json:"tenantId"`
		Transaction string `json:"transaction"`
		Parcel      string `json:"parcel"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return LabelTransactionJudgmentDue{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Transaction == "" || body.Parcel == "" {
		return LabelTransactionJudgmentDue{}, fmt.Errorf("%w: missing judgment key fields", ErrPoisonEnvelope)
	}
	return LabelTransactionJudgmentDue{
		TenantID:    body.TenantID,
		Transaction: body.Transaction,
		Parcel:      body.Parcel,
	}, nil
}
