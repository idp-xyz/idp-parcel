package nrinbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// transportHandoverConsumerName 是本消费者在 inbox 键上的稳定名。同一封交接信封还扇出给追踪投影与派送发起两路，inbox
// 键只由（消费者名 + 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。与收寄采用那一路也分家：两条复核
// 触发各记各的账。改名等于换消费者。
const transportHandoverConsumerName = "network-routing/reassess-on-transport-handover"

// TransportHandoverRegisteredEventType 是本消费者认的事件类型：transport-fulfillment 的权威交接登记（ADR-0215）。
// 串在本包写出，不导入提供方 outbox 适配器的常量，理由同 AcceptedDecisionEventType；它与提供方交出的串相等，由
// cmd/parcel-dispatch 用生产交接口入队的真库用例钉住。
const TransportHandoverRegisteredEventType eventing.EventType = "transport-fulfillment.transport-handover.registered"

// RegisteredTransportHandover 是译码后的交接判断键，四维都是 TF 交接登记册的键维。交接本体与它声明的节点由处理方
// 按键读回：信封只带键（ADR-0213 决定三），载荷里带一份节点会让「交接进了哪个节点」在两处定义。版本必须进译码——
// 更正是新版本新登记，少了版本就读不回是哪一版。
type RegisteredTransportHandover struct {
	TenantID string
	Object   string
	Scope    string
	Version  string
}

// TransportHandoverHandler 是本消费者转交的处理方。真实装配接 NR 交接触发的复核适配器；消费门只保证「处理成功与
// 消费入账同一事务」。
type TransportHandoverHandler interface {
	HandleRegisteredTransportHandover(ctx context.Context, registered RegisteredTransportHandover) error
}

// TransportHandoverConsumer 把 TF 权威交接登记信封推进消费门，交给交接触发的路由复核（UC-NR-003，ADR-0215）。
type TransportHandoverConsumer struct {
	gate *inboxconsume.Gate[RegisteredTransportHandover]
}

func NewTransportHandoverConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler TransportHandoverHandler,
) (*TransportHandoverConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("network routing inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("network routing inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("network routing inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredTransportHandover]{
		Transactor:     transactor,
		Store:          store,
		Name:           transportHandoverConsumerName,
		EventType:      TransportHandoverRegisteredEventType,
		Decode:         decodeRegisteredTransportHandover,
		Handle:         handler.HandleRegisteredTransportHandover,
		UnexpectedType: "network routing inbox",
		HandleVerb:     "handle registered transport handover",
	})
	if err != nil {
		return nil, err
	}
	return &TransportHandoverConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume，与本包另两扇门共用。
func (consumer *TransportHandoverConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRegisteredTransportHandover 译载荷。四维缺一即毒丸：处理方按整键取回交接，缺任一维都取不着那一行，重投同样
// 内容也不会长出字段来。
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
