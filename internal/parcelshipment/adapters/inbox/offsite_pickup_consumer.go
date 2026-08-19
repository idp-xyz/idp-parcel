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

// offsitePickupConsumerName 是本消费者在 inbox 键上的稳定名。与节点收寄那本账分开：
// Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，两路共名会让一路把另一路的投递当
// 重复跳过。改名等于换消费者。
const offsitePickupConsumerName = "parcel-shipment/adopt-offsite-pickup"

// OffsitePickupRegisteredEventType 是本消费者认的事件类型：TF 的**对象级**揽收登记。
// 不认尝试级的 `offsite-pickup.formed`——那一封信带一批成功对象，而采用判断逐对象
// 成立，一封信一对象才与消费门的入账/回滚两格对得上。这条纪律只在存在对象级替代品
// 时起选择作用；没有替代品的多成员事实按 ADR-0066 在消费侧循环拆分，不受此句约束。
const OffsitePickupRegisteredEventType eventing.EventType = "transport-fulfillment.offsite-pickup.registered"

// RegisteredOffsitePickup 是译码后的揽收登记幂等键引用——只有引用，揽收本体由处理方
// 按引用重新取（权威事实留在 transport-fulfillment）。
type RegisteredOffsitePickup struct {
	TenantID string
	Object   string
	Attempt  string
}

// RegisteredOffsitePickupHandler 是本消费者转交的处理方。真实装配接
// AdoptOnOffsitePickupAdapter。
type RegisteredOffsitePickupHandler interface {
	HandleRegisteredOffsitePickup(ctx context.Context, registered RegisteredOffsitePickup) error
}

// OffsitePickupConsumer 把 TF 对象级揽收登记信封推进消费门。
type OffsitePickupConsumer struct {
	gate *inboxconsume.Gate[RegisteredOffsitePickup]
}

func NewOffsitePickupConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RegisteredOffsitePickupHandler,
) (*OffsitePickupConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel shipment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredOffsitePickup]{
		Transactor:     transactor,
		Store:          store,
		Name:           offsitePickupConsumerName,
		EventType:      OffsitePickupRegisteredEventType,
		Decode:         decodeRegisteredOffsitePickup,
		Handle:         handler.HandleRegisteredOffsitePickup,
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "handle registered offsite pickup",
	})
	if err != nil {
		return nil, err
	}
	return &OffsitePickupConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余消费者共用同一扇门。
func (consumer *OffsitePickupConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRegisteredOffsitePickup 译载荷。三维缺一即毒丸——处理方按（租户+对象+尝试）
// 取回登记，缺了永远取不着，而重投同样内容不会长出字段来。
func decodeRegisteredOffsitePickup(payload []byte) (RegisteredOffsitePickup, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Object   string `json:"object"`
		Attempt  string `json:"attempt"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredOffsitePickup{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Object == "" || body.Attempt == "" {
		return RegisteredOffsitePickup{}, fmt.Errorf("%w: missing pickup key fields", ErrPoisonEnvelope)
	}
	return RegisteredOffsitePickup{
		TenantID: body.TenantID,
		Object:   body.Object,
		Attempt:  body.Attempt,
	}, nil
}
