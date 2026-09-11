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

// carrierFirstEffectivePickupConsumerName 是本消费者在 inbox 键上的稳定名。与交付 / 揽收 / 面单交易 / 关闭重开
// 各路分开：Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const carrierFirstEffectivePickupConsumerName = "parcel-shipment/judge-label-final-from-carrier-first-effective-pickup"

// CarrierFirstEffectivePickupRegisteredEventType 是本消费者认的事件类型：TF 就一个载运对象登记一版**已形成 / 替代 /
// 失效**的实际承运商首次有效收寄（ADR-0135 决定七；待确认版本不入队）。消费方自己写出这个串，不导入提供方
// outbox 适配器的未导出常量。
//
// 不认 `external-carrier-tracking.judged`：外部承运轨迹事实按 TF CONTEXT「本身不构成实际承运商首次有效收寄」，
// 这扇门吃了它就是 PS 替 TF 判「这条状态词算收寄」（票 label-channel/25 红线）。
const CarrierFirstEffectivePickupRegisteredEventType eventing.EventType = "transport-fulfillment.carrier-first-effective-pickup.registered"

// RegisteredCarrierFirstEffectivePickup 是译码后的指针：（租户 + 事实 + 版本）是取回那一代的键，Object 是提供方随
// 载荷带出的载运对象。本体由处理方按键取回**指名那一代**（权威事实留在 transport-fulfillment）——版本在键里而不在
// 事件 ID 里独占，是因为一条链的替代 / 失效版本走同一个事实身份，按当前版读会把更正之前入队的那一份也读成
// 更正后那一代（票 label-channel/24 的教训）。Object 不是键的一维：缺了处理方从本体读，带了用来核信封与本体一致。
type RegisteredCarrierFirstEffectivePickup struct {
	TenantID string
	Fact     string
	Version  string
	Object   string
}

// RegisteredCarrierFirstEffectivePickupHandler 是本消费者转交的处理方。真实装配接
// adapters/transportfulfillment 的 JudgeOnCarrierFirstEffectivePickupAdapter。
type RegisteredCarrierFirstEffectivePickupHandler interface {
	HandleRegisteredCarrierFirstEffectivePickup(ctx context.Context, registered RegisteredCarrierFirstEffectivePickup) error
}

// CarrierFirstEffectivePickupConsumer 把 TF 首次有效收寄登记信封推进消费门。
type CarrierFirstEffectivePickupConsumer struct {
	gate *inboxconsume.Gate[RegisteredCarrierFirstEffectivePickup]
}

func NewCarrierFirstEffectivePickupConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RegisteredCarrierFirstEffectivePickupHandler,
) (*CarrierFirstEffectivePickupConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("parcel shipment inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("parcel shipment inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("parcel shipment inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredCarrierFirstEffectivePickup]{
		Transactor:     transactor,
		Store:          store,
		Name:           carrierFirstEffectivePickupConsumerName,
		EventType:      CarrierFirstEffectivePickupRegisteredEventType,
		Decode:         decodeRegisteredCarrierFirstEffectivePickup,
		Handle:         handler.HandleRegisteredCarrierFirstEffectivePickup,
		UnexpectedType: "parcel shipment inbox",
		HandleVerb:     "handle registered carrier first effective pickup",
	})
	if err != nil {
		return nil, err
	}
	return &CarrierFirstEffectivePickupConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与其余消费者共用同一扇门。
func (consumer *CarrierFirstEffectivePickupConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRegisteredCarrierFirstEffectivePickup 译载荷。键三维缺一即毒丸——处理方按（租户 + 事实 + 版本）取回
// 那一代，缺了永远取不着，而重投同样内容不会长出字段来。object 不在毒丸判据里：它不是键，处理方能从本体读回。
func decodeRegisteredCarrierFirstEffectivePickup(payload []byte) (RegisteredCarrierFirstEffectivePickup, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Fact     string `json:"fact"`
		Version  string `json:"version"`
		Object   string `json:"object"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredCarrierFirstEffectivePickup{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Fact == "" || body.Version == "" {
		return RegisteredCarrierFirstEffectivePickup{}, fmt.Errorf("%w: missing carrier pickup key fields", ErrPoisonEnvelope)
	}
	return RegisteredCarrierFirstEffectivePickup{
		TenantID: body.TenantID,
		Fact:     body.Fact,
		Version:  body.Version,
		Object:   body.Object,
	}, nil
}
