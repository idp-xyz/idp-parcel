package sainbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// chargeOccurrenceFormedConsumerName 是本消费者在 inbox 键上的稳定名。
const chargeOccurrenceFormedConsumerName = "settlement-accounting/request-buy-evaluation-on-occurrence-formed"

// ChargeOccurrenceFormedEventType 是本消费者认的事件类型：运输履约登记一份运输收费发生项之后交出的信封（ADR-0208）。
// 字符串由消费方自己写出。生产方上线之前本消费者先登路由（ADR-0049：某类事件的生产方上线之前，它的消费者必须先在路由
// 表里）；生产方上线那一笔用它的真适配器入队，在 cmd/parcel-dispatch 的真库装配用例里钉两串相等。
const ChargeOccurrenceFormedEventType eventing.EventType = "transport-fulfillment.charge-occurrence.formed"

// FormedChargeOccurrence 是译码后的发生项引用，恰是运输履约登记册的键（租户、发生项、有效性版本）。原因、业务时间、
// 协议、成员一样都不收：它们只有登记册一处权威，处理方按键回读，信封上抄一份就是第二处。
type FormedChargeOccurrence struct {
	TenantID   string
	Occurrence string
	Version    string
}

// FormedChargeOccurrenceHandler 是本消费者转交的处理方。真实装配接 adapters/transportfulfillment 的
// RequestOnChargeOccurrenceFormedAdapter。
type FormedChargeOccurrenceHandler interface {
	HandleFormedChargeOccurrence(ctx context.Context, formed FormedChargeOccurrence) error
}

// ChargeOccurrenceFormedConsumer 把发生项已形成信封推进消费门。
type ChargeOccurrenceFormedConsumer struct {
	gate *inboxconsume.Gate[FormedChargeOccurrence]
}

func NewChargeOccurrenceFormedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedChargeOccurrenceHandler,
) (*ChargeOccurrenceFormedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("settlement accounting inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedChargeOccurrence]{
		Transactor:     transactor,
		Store:          store,
		Name:           chargeOccurrenceFormedConsumerName,
		EventType:      ChargeOccurrenceFormedEventType,
		Decode:         decodeFormedChargeOccurrence,
		Handle:         handler.HandleFormedChargeOccurrence,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle formed charge occurrence",
	})
	if err != nil {
		return nil, err
	}
	return &ChargeOccurrenceFormedConsumer{gate: gate}, nil
}

func (consumer *ChargeOccurrenceFormedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedChargeOccurrence 译载荷。三维缺一即毒丸：处理方按整键回读，缺一维永远取不着那一行。版本尤其不给缺省
// ——更正换版本、两代共存，按两维读「当前版」会把更正与原发生项叠成一次查找。
func decodeFormedChargeOccurrence(payload []byte) (FormedChargeOccurrence, error) {
	var body struct {
		TenantID   string `json:"tenantId"`
		Occurrence string `json:"occurrence"`
		Version    string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedChargeOccurrence{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Occurrence == "" || body.Version == "" {
		return FormedChargeOccurrence{}, fmt.Errorf("%w: missing charge occurrence key fields", ErrPoisonEnvelope)
	}
	return FormedChargeOccurrence{
		TenantID:   body.TenantID,
		Occurrence: body.Occurrence,
		Version:    body.Version,
	}, nil
}
