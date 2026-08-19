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

// transportHandoverConsumerName 是本消费者在 inbox 键上的稳定名。必须与交付、揽收
// 那两本账分家：Inbox 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路
// 的投递当重复跳过。改名等于换消费者。
const transportHandoverConsumerName = "visibility-exception/derive-projection-from-transport-handover"

// TransportHandoverRegisteredEventType 是本消费者认的事件类型：TF 的权威交接登记。
// 消费方自己写出这个字符串，不导入提供方 outbox 适配器的未导出常量。
//
// 不认 `effective-delivery.registered`、不认 `offsite-pickup.registered` / `.formed`
// ——那些各有自己的投影账。
const TransportHandoverRegisteredEventType eventing.EventType = "transport-fulfillment.transport-handover.registered"

// RegisteredTransportHandover 是译码后的交接判断幂等键引用。权威事实留在
// transport-fulfillment；处理方按这四维 FindByKey。版本是键的一维（更正是新版本新
// 登记），必须进译码——这与有效交付不同：交付把版本放进事件 ID、载荷只带三维键。
type RegisteredTransportHandover struct {
	TenantID string
	Object   string
	Scope    string
	Version  string
}

// RegisteredTransportHandoverHandler 是本消费者转交的处理方。真实装配接
// DeriveOnTransportHandoverAdapter。
type RegisteredTransportHandoverHandler interface {
	HandleRegisteredTransportHandover(ctx context.Context, registered RegisteredTransportHandover) error
}

// TransportHandoverConsumer 把 TF 权威交接登记信封推进消费门。
type TransportHandoverConsumer struct {
	gate *inboxconsume.Gate[RegisteredTransportHandover]
}

func NewTransportHandoverConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RegisteredTransportHandoverHandler,
) (*TransportHandoverConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredTransportHandover]{
		Transactor:     transactor,
		Store:          store,
		Name:           transportHandoverConsumerName,
		EventType:      TransportHandoverRegisteredEventType,
		Decode:         decodeRegisteredTransportHandover,
		Handle:         handler.HandleRegisteredTransportHandover,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle registered transport handover",
	})
	if err != nil {
		return nil, err
	}
	return &TransportHandoverConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *TransportHandoverConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRegisteredTransportHandover 译载荷。四维（含 version）缺一即毒丸——处理方按
// （租户+对象+范围+版本）取回登记，缺了永远取不着，而重投同样内容不会长出字段来。
// 毒丸哨兵复用本包已有的 ErrPoisonEnvelope。
func decodeRegisteredTransportHandover(payload []byte) (RegisteredTransportHandover, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Object   string `json:"object"`
		Scope    string `json:"scope"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RegisteredTransportHandover{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Object == "" || body.Scope == "" || body.Version == "" {
		return RegisteredTransportHandover{}, fmt.Errorf("%w: missing handover key fields", ErrPoisonEnvelope)
	}
	return RegisteredTransportHandover{
		TenantID: body.TenantID,
		Object:   body.Object,
		Scope:    body.Scope,
		Version:  body.Version,
	}, nil
}
