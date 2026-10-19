package veinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// externalResultConsumerName 是本消费者在 inbox 键上的稳定名。Inbox 键只由
// （消费者名 + 来源 + 事件 ID）认领，与同源案件、申报提交两本账分开；改名等于换消费者。
const externalResultConsumerName = "visibility-exception/derive-projection-from-external-result"

// ExternalResultReceivedEventType 是本消费者认的事件类型：CC 的外部监管结果已接收。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 一封信是一次接收形成的一层外部结果；结果范围指名申报单元，成员由处理方经范围→单元
// 取回，按 ADR-0066 在消费侧逐成员拆分。
const ExternalResultReceivedEventType eventing.EventType = "customs-compliance.external-result.received"

// ReceivedExternalResult 是译码后的接收幂等键引用——只有引用，层次、范围与放行三件由
// 处理方按键重读本体取回（权威事实留在 customs-compliance）。
type ReceivedExternalResult struct {
	TenantID string
	SourceID string
}

// ReceivedExternalResultHandler 是本消费者转交的处理方。真实装配接
// DeriveOnExternalResultAdapter。
type ReceivedExternalResultHandler interface {
	HandleReceivedExternalResult(ctx context.Context, received ReceivedExternalResult) error
}

// ExternalResultConsumer 把 CC 外部结果已接收信封推进消费门。
type ExternalResultConsumer struct {
	gate *inboxconsume.Gate[ReceivedExternalResult]
}

func NewExternalResultConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler ReceivedExternalResultHandler,
) (*ExternalResultConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[ReceivedExternalResult]{
		Transactor:     transactor,
		Store:          store,
		Name:           externalResultConsumerName,
		EventType:      ExternalResultReceivedEventType,
		Decode:         decodeReceivedExternalResult,
		Handle:         handler.HandleReceivedExternalResult,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle received external result",
	})
	if err != nil {
		return nil, err
	}
	return &ExternalResultConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *ExternalResultConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeReceivedExternalResult 译载荷。接收幂等键两维缺一即毒丸——缺了处理方永远取
// 不着，重投同样内容也不会长出字段来。
func decodeReceivedExternalResult(payload []byte) (ReceivedExternalResult, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		SourceID string `json:"sourceId"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return ReceivedExternalResult{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.SourceID == "" {
		return ReceivedExternalResult{}, fmt.Errorf("%w: missing external result key fields", ErrPoisonEnvelope)
	}
	return ReceivedExternalResult{TenantID: body.TenantID, SourceID: body.SourceID}, nil
}
