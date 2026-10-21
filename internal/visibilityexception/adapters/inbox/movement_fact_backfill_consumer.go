package veinbox

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// movementFactBackfillOnHandoverConsumerName 是交接信封上移动事实补派生那一路的 inbox 名（ADR-0224）。与交接投影那一本账分家：
// 两路认同一封信，共名会让先到的一路把另一路的投递当重复跳过，补派生就永远不跑。改名等于换消费者。
const movementFactBackfillOnHandoverConsumerName = "visibility-exception/backfill-movement-facts-on-transport-handover"

// MovementFactBackfillOnHandoverConsumer 把 TF 权威交接登记信封推进补派生那一路的消费门。信封与译码同交接投影那一路
// （TransportHandoverConsumer），只是账与处理方各是各的。
type MovementFactBackfillOnHandoverConsumer struct {
	gate *inboxconsume.Gate[RegisteredTransportHandover]
}

func NewMovementFactBackfillOnHandoverConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RegisteredTransportHandoverHandler,
) (*MovementFactBackfillOnHandoverConsumer, error) {
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
		Name:           movementFactBackfillOnHandoverConsumerName,
		EventType:      TransportHandoverRegisteredEventType,
		Decode:         decodeRegisteredTransportHandover,
		Handle:         handler.HandleRegisteredTransportHandover,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "backfill movement facts on transport handover",
	})
	if err != nil {
		return nil, err
	}
	return &MovementFactBackfillOnHandoverConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *MovementFactBackfillOnHandoverConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}
