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

// departureGateReaskConsumerName 是本消费者在 inbox 键上的稳定名，与移动事实那一本账分家。改名等于换消费者。
const departureGateReaskConsumerName = "visibility-exception/raise-departure-gate-signal-from-reask"

// DepartureGateReaskRecordedEventType 是本消费者认的事件类型：TF 给一条出发追加了一次门禁补问读数（ADR-0305）。消费方自己写出
// 这个字符串，不导入提供方 outbox 适配器的常量。
const DepartureGateReaskRecordedEventType eventing.EventType = "transport-fulfillment.departure-gate-reask.recorded"

// RecordedDepartureGateReask 是译码后的补问键引用，五维都是 TF 补问登记册的键维；读数只在那一本册上，处理方按键回读。
type RecordedDepartureGateReask struct {
	TenantID string
	Fact     string
	Version  string
	Object   string
	Sequence string
}

// RecordedDepartureGateReaskHandler 是本消费者转交的处理方。真实装配接 RaiseOnDepartureGateReaskAdapter。
type RecordedDepartureGateReaskHandler interface {
	HandleRecordedDepartureGateReask(ctx context.Context, recorded RecordedDepartureGateReask) error
}

// DepartureGateReaskConsumer 把 TF 出发门禁补问信封推进消费门。
type DepartureGateReaskConsumer struct {
	gate *inboxconsume.Gate[RecordedDepartureGateReask]
}

func NewDepartureGateReaskConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler RecordedDepartureGateReaskHandler,
) (*DepartureGateReaskConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("visibility exception inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("visibility exception inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("visibility exception inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[RecordedDepartureGateReask]{
		Transactor:     transactor,
		Store:          store,
		Name:           departureGateReaskConsumerName,
		EventType:      DepartureGateReaskRecordedEventType,
		Decode:         decodeRecordedDepartureGateReask,
		Handle:         handler.HandleRecordedDepartureGateReask,
		UnexpectedType: "visibility exception inbox",
		HandleVerb:     "handle recorded departure gate reask",
	})
	if err != nil {
		return nil, err
	}
	return &DepartureGateReaskConsumer{gate: gate}, nil
}

// Consume 处理一份投递。舞步在 inboxconsume。
func (consumer *DepartureGateReaskConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeRecordedDepartureGateReask 译载荷。五维缺一即毒丸：处理方按这五维取回补问，缺一维永远取不着。
func decodeRecordedDepartureGateReask(payload []byte) (RecordedDepartureGateReask, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Fact     string `json:"fact"`
		Version  string `json:"version"`
		Object   string `json:"object"`
		Sequence string `json:"sequence"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return RecordedDepartureGateReask{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Fact == "" || body.Version == "" || body.Object == "" || body.Sequence == "" {
		return RecordedDepartureGateReask{}, fmt.Errorf("%w: missing departure gate reask key fields", ErrPoisonEnvelope)
	}
	return RecordedDepartureGateReask{
		TenantID: body.TenantID, Fact: body.Fact, Version: body.Version, Object: body.Object, Sequence: body.Sequence,
	}, nil
}
