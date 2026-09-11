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

// continuedAttemptDecisionJudgmentConsumerName 是本消费者在 inbox 键上的稳定名。与面单交易判断意图那一路
// （lc/26）、交付 / 收寄 / 揽收各路分开：Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把
// 另一路的投递当重复跳过。改名等于换消费者。
const continuedAttemptDecisionJudgmentConsumerName = "parcel-shipment/judge-label-final-from-continued-attempt-decision"

// ContinuedAttemptDecisionJudgmentDueEventType 是本消费者认的事件类型：`面单继续尝试决定`写侧在关闭 / 重开
// 决定落册（`Insert` / `Save` 成功）后交出的「这件包裹值得判一次终局」指针式信封（ADR-0134 决定一）。两种
// 决定共用一个类型，一扇门一个处理方。消费方按本包惯例自己写出这个串，不 import 提供方的 postgres 适配器
// （那一侧为本消费者导出了同一个串，两串相等由本包测试钉住）。
const ContinuedAttemptDecisionJudgmentDueEventType eventing.EventType = "parcel-shipment.continued-attempt-decision.judgment-due"

// ContinuedAttemptDecisionJudgmentDue 是译码后的指针：租户、要判的包裹、触发的决定。**不带决定本体，处理方
// 也不按它读回登记册**——判断读全册且读当下（CONTEXT「必须跨该包裹全部相关交易及实际承运商收寄事实形成
// 包裹级判断」），决定标识只用于幂等与追溯。载荷里的 kind 故意不进这里：两种决定都触发、分格归
// JudgeLabelServiceFinal，消费者不据种类分支（票 label-channel/27 红线「不在触发处按决定种类挑」）。
type ContinuedAttemptDecisionJudgmentDue struct {
	TenantID string
	Parcel   string
	Decision string
}

// ContinuedAttemptDecisionJudgmentDueHandler 是本消费者转交的处理方。真实装配接三路共用的处理方核
// （adapters/labelfinal）。
type ContinuedAttemptDecisionJudgmentDueHandler interface {
	HandleContinuedAttemptDecisionJudgmentDue(ctx context.Context, due ContinuedAttemptDecisionJudgmentDue) error
}

// ContinuedAttemptDecisionJudgmentConsumer 把关闭 / 重开决定判断意图信封推进消费门。
type ContinuedAttemptDecisionJudgmentConsumer struct {
	gate *inboxconsume.Gate[ContinuedAttemptDecisionJudgmentDue]
}

func NewContinuedAttemptDecisionJudgmentConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler ContinuedAttemptDecisionJudgmentDueHandler,
) (*ContinuedAttemptDecisionJudgmentConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel shipment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[ContinuedAttemptDecisionJudgmentDue]{
		Transactor:     transactor,
		Store:          store,
		Name:           continuedAttemptDecisionJudgmentConsumerName,
		EventType:      ContinuedAttemptDecisionJudgmentDueEventType,
		Decode:         decodeContinuedAttemptDecisionJudgmentDue,
		Handle:         handler.HandleContinuedAttemptDecisionJudgmentDue,
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "handle continued attempt decision judgment due",
	})
	if err != nil {
		return nil, err
	}
	return &ContinuedAttemptDecisionJudgmentConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余消费者共用同一扇门。
func (consumer *ContinuedAttemptDecisionJudgmentConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeContinuedAttemptDecisionJudgmentDue 译载荷。三维缺一即毒丸——处理方按（租户 + 包裹）反查目标委托、
// 按决定标识追溯，缺了永远补不上，而重投同样内容不会长出字段来。kind 故意不读。
func decodeContinuedAttemptDecisionJudgmentDue(payload []byte) (ContinuedAttemptDecisionJudgmentDue, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Parcel   string `json:"parcel"`
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ContinuedAttemptDecisionJudgmentDue{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Decision == "" {
		return ContinuedAttemptDecisionJudgmentDue{}, fmt.Errorf("%w: missing judgment key fields", ErrPoisonEnvelope)
	}
	return ContinuedAttemptDecisionJudgmentDue{
		TenantID: body.TenantID,
		Parcel:   body.Parcel,
		Decision: body.Decision,
	}, nil
}
