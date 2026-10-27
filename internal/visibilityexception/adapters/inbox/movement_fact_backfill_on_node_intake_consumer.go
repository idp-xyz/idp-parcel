package veinbox

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
)

// movementFactBackfillOnNodeIntakeConsumerName 是节点收寄形成信封上移动事实补派生的 inbox 名（ADR-0299）。与同一封信上的投影那一本账
// （nodeIntakeConsumerName）和 parcel-shipment 的来源采用分家：几路认同一封信，共名会让先到的一路把另一路的投递当重复跳过，补派生
// 就永远不跑。改名等于换消费者。
const movementFactBackfillOnNodeIntakeConsumerName = "visibility-exception/backfill-movement-facts-on-node-intake"

// MovementFactBackfillOnNodeIntakeConsumer 把 NO 节点收寄形成信封推进补派生那一路的消费门。信封与译码同收寄投影那一路
// （NodeIntakeConsumer），只是账与处理方各是各的；信封载荷不加字段，别的订阅方的合同一字不动。
type MovementFactBackfillOnNodeIntakeConsumer struct {
	gate *inboxconsume.Gate[FormedNodeIntake]
}

func NewMovementFactBackfillOnNodeIntakeConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedNodeIntakeHandler,
) (*MovementFactBackfillOnNodeIntakeConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedNodeIntake]{
		Transactor:     transactor,
		Store:          store,
		Name:           movementFactBackfillOnNodeIntakeConsumerName,
		EventType:      NodeIntakeFormedEventType,
		Decode:         decodeFormedNodeIntake,
		Handle:         handler.HandleFormedNodeIntake,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "backfill movement facts on node intake",
	})
	if err != nil {
		return nil, err
	}
	return &MovementFactBackfillOnNodeIntakeConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *MovementFactBackfillOnNodeIntakeConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}
