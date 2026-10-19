package noinbox

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// transferControlOnHandoverConsumerName 是本消费者在 inbox 键上的稳定名。同一封交接信封还扇出给别的消费方，inbox 键只由（消费者名 +
// 来源 + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const transferControlOnHandoverConsumerName = "node-operations/transfer-control-on-transport-handover"

// TransportHandoverRegisteredEventType 是 transport-fulfillment 权威运输交接登记成立时交出的信封（ADR-0214 决定一）。消费方自己写出
// 这个字符串，不导入提供方 outbox 适配器的常量；与生产交出的是同一个串，由 cmd/parcel-dispatch 的一条用例经派发一拍真落到本消费者
// 名下钉住。
const TransportHandoverRegisteredEventType eventing.EventType = "transport-fulfillment.transport-handover.registered"

// RegisteredTransportHandover 是译码后的交接判断键，四维都是 TF 登记册的键维，原样交给消费侧适配器去读回那一版。版本必须进译码：
// 更正是新版本新登记，少了版本就读不回是哪一版。
type RegisteredTransportHandover struct {
	TenantID string
	Object   string
	Scope    string
	Version  string
}

// RegisteredTransportHandoverHandler 是本消费门转交的处理方。真实装配接 adapters/transportfulfillment 的
// TransferControlOnHandoverAdapter：跨上下文的读回与翻译在那里（ADR-0025），本包只认信封。
type RegisteredTransportHandoverHandler interface {
	HandleRegisteredTransportHandover(ctx context.Context, registered RegisteredTransportHandover) error
}

// TransportHandoverRegisteredConsumer 把交接登记信封推进消费门，交给节点控制的转出与转入（UC-NO-005）。处理方交回错误整笔回滚，
// inbox 无账，信封退避重投。
type TransportHandoverRegisteredConsumer struct {
	gate *inboxconsume.Gate[RegisteredTransportHandover]
}

func NewTransportHandoverRegisteredConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RegisteredTransportHandoverHandler,
) (*TransportHandoverRegisteredConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("node operations inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("node operations inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("node operations inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RegisteredTransportHandover]{
		Transactor:     transactor,
		Store:          store,
		Name:           transferControlOnHandoverConsumerName,
		EventType:      TransportHandoverRegisteredEventType,
		Decode:         decodeRegisteredTransportHandover,
		Handle:         handler.HandleRegisteredTransportHandover,
		UnexpectedType: "node operations inbox",
		HandleVerb:     "transfer node control on registered transport handover",
	})
	if err != nil {
		return nil, err
	}
	return &TransportHandoverRegisteredConsumer{gate: gate}, nil
}

func (consumer *TransportHandoverRegisteredConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRegisteredTransportHandover 译载荷。四维缺一或空白即毒丸：处理方按（租户 + 对象 + 范围 + 版本）读回交接，缺一维永远取不着，
// 重投同样内容也不会长出字段来。载荷里另有的格不在这里读：节点控制只认按键读回的那一版。
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
	for _, field := range []string{body.TenantID, body.Object, body.Scope, body.Version} {
		if strings.TrimSpace(field) == "" {
			return RegisteredTransportHandover{}, fmt.Errorf("%w: missing transport handover key fields", ErrPoisonEnvelope)
		}
	}
	return RegisteredTransportHandover{
		TenantID: body.TenantID,
		Object:   body.Object,
		Scope:    body.Scope,
		Version:  body.Version,
	}, nil
}
