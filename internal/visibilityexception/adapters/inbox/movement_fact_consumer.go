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

// movementFactConsumerName 是本消费者在 inbox 键上的稳定名。与交接、交付、揽收各本账分家：Inbox 键只由（消费者名 + 来源
// + 事件 ID）认领，共名会让一路把另一路的投递当重复跳过。改名等于换消费者。
const movementFactConsumerName = "visibility-exception/derive-projection-from-movement-fact"

// MovementFactRecordedEventType 是本消费者认的事件类型：TF 登记了一个实际移动事实版本（ADR-0205 决定五）。消费方自己写出
// 这个字符串，不导入提供方 outbox 适配器的常量。
const MovementFactRecordedEventType eventing.EventType = "transport-fulfillment.movement-fact.recorded"

// RecordedMovementFact 是译码后的移动事实键引用。权威事实留在 transport-fulfillment；处理方按这三维 FindByKey。版本是键的
// 一维（迟到与更正是新版本新登记），必须进译码。
type RecordedMovementFact struct {
	TenantID string
	Fact     string
	Version  string
}

// RecordedMovementFactHandler 是本消费者转交的处理方。真实装配接 DeriveOnMovementFactAdapter。
type RecordedMovementFactHandler interface {
	HandleRecordedMovementFact(ctx context.Context, recorded RecordedMovementFact) error
}

// MovementFactConsumer 把 TF 移动事实已形成信封推进消费门。
type MovementFactConsumer struct {
	gate *inboxconsume.Gate[RecordedMovementFact]
}

func NewMovementFactConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RecordedMovementFactHandler,
) (*MovementFactConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedMovementFact]{
		Transactor:     transactor,
		Store:          store,
		Name:           movementFactConsumerName,
		EventType:      MovementFactRecordedEventType,
		Decode:         decodeRecordedMovementFact,
		Handle:         handler.HandleRecordedMovementFact,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle recorded movement fact",
	})
	if err != nil {
		return nil, err
	}
	return &MovementFactConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *MovementFactConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedMovementFact 译载荷。三维缺一即毒丸——处理方按（租户+事实+版本）取回登记，缺了永远取不着，而重投同样内容
// 不会长出字段来。
func decodeRecordedMovementFact(payload []byte) (RecordedMovementFact, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Fact     string `json:"fact"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedMovementFact{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Fact == "" || body.Version == "" {
		return RecordedMovementFact{}, fmt.Errorf("%w: missing movement fact key fields", ErrPoisonEnvelope)
	}
	return RecordedMovementFact{TenantID: body.TenantID, Fact: body.Fact, Version: body.Version}, nil
}
