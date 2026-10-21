package sainbox

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// chargeOccurrenceInvalidatedConsumerName 是本消费者在 inbox 键上的稳定名：与发起 BUY 评价那一路消费同一类信封，各记各的账。
const chargeOccurrenceInvalidatedConsumerName = "settlement-accounting/withdraw-supplier-cost-on-occurrence-invalidated"

// InvalidatedChargeOccurrenceHandler 是本消费者转交的处理方：按键回读，只对失效版本动手（ADR-0232 决定七）。真实装配接
// adapters/transportfulfillment 的 WithdrawOnChargeOccurrenceInvalidatedAdapter。
type InvalidatedChargeOccurrenceHandler interface {
	HandleInvalidatedChargeOccurrence(ctx context.Context, formed FormedChargeOccurrence) error
}

// ChargeOccurrenceInvalidatedConsumer 把发生项已形成信封推进消费门，交给撤下预期成本的那一路。信封类型与载荷同 ChargeOccurrenceFormedConsumer：
// 运输履约每长出一版（首版、失效版、替代版）都交同一类信封，是不是失效版要按键回读才知道。
type ChargeOccurrenceInvalidatedConsumer struct {
	gate *inboxconsume.Gate[FormedChargeOccurrence]
}

func NewChargeOccurrenceInvalidatedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler InvalidatedChargeOccurrenceHandler,
) (*ChargeOccurrenceInvalidatedConsumer, error) {
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
		Name:           chargeOccurrenceInvalidatedConsumerName,
		EventType:      ChargeOccurrenceFormedEventType,
		Decode:         decodeFormedChargeOccurrence,
		Handle:         handler.HandleInvalidatedChargeOccurrence,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "withdraw supplier cost on invalidated charge occurrence",
	})
	if err != nil {
		return nil, err
	}
	return &ChargeOccurrenceInvalidatedConsumer{gate: gate}, nil
}

func (consumer *ChargeOccurrenceInvalidatedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}
