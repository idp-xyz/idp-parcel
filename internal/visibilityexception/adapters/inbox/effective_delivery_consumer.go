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

// effectiveDeliveryConsumerName 是本消费者在 inbox 键上的稳定名。必须与
// parcel-shipment/form-final-from-delivery 分账：Inbox 键只由（消费者名 + 来源 + 事件 ID）
// 认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const effectiveDeliveryConsumerName = "visibility-exception/derive-projection-from-effective-delivery"

// EffectiveDeliveryRegisteredEventType 是本消费者认的事件类型：TF 的有效交付登记。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 不认 `transport-handover.registered`（控制转出，属 node-operations）、不认
// `offsite-pickup.formed` / `.registered`（揽收投影另账）——那些都不是有效交付事实。
const EffectiveDeliveryRegisteredEventType eventing.EventType = "transport-fulfillment.effective-delivery.registered"

// RegisteredEffectiveDelivery 是译码后的有效交付幂等键引用——只有引用，交付本体由
// 处理方按引用重新取（权威事实留在 transport-fulfillment）。结果版本不在这里：提供方
// 把版本放进事件 ID 以区分两代入队，载荷只带键，处理方 FindByKey 读当前版本。
type RegisteredEffectiveDelivery struct {
	TenantID string
	Object   string
	Attempt  string
}

// RegisteredEffectiveDeliveryHandler 是本消费者转交的处理方。真实装配接
// DeriveOnEffectiveDeliveryAdapter。
type RegisteredEffectiveDeliveryHandler interface {
	HandleRegisteredEffectiveDelivery(ctx context.Context, registered RegisteredEffectiveDelivery) error
}

// EffectiveDeliveryConsumer 把 TF 有效交付登记信封推进消费门。
type EffectiveDeliveryConsumer struct {
	gate *inboxconsume.Gate[RegisteredEffectiveDelivery]
}

func NewEffectiveDeliveryConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RegisteredEffectiveDeliveryHandler,
) (*EffectiveDeliveryConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredEffectiveDelivery]{
		Transactor:     transactor,
		Store:          store,
		Name:           effectiveDeliveryConsumerName,
		EventType:      EffectiveDeliveryRegisteredEventType,
		Decode:         decodeRegisteredEffectiveDelivery,
		Handle:         handler.HandleRegisteredEffectiveDelivery,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle registered effective delivery",
	})
	if err != nil {
		return nil, err
	}
	return &EffectiveDeliveryConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *EffectiveDeliveryConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRegisteredEffectiveDelivery 译载荷。三维缺一即毒丸——处理方按（租户+对象+尝试）
// 取回登记，缺了永远取不着，而重投同样内容不会长出字段来。多余的 version 字段故意不读：
// 那是事件 ID 的事，不是引用维。毒丸哨兵复用本包已有的 ErrPoisonEnvelope。
func decodeRegisteredEffectiveDelivery(payload []byte) (RegisteredEffectiveDelivery, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Object   string `json:"object"`
		Attempt  string `json:"attempt"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredEffectiveDelivery{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Object == "" || body.Attempt == "" {
		return RegisteredEffectiveDelivery{}, fmt.Errorf("%w: missing delivery key fields", ErrPoisonEnvelope)
	}
	return RegisteredEffectiveDelivery{
		TenantID: body.TenantID,
		Object:   body.Object,
		Attempt:  body.Attempt,
	}, nil
}
