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

// finalOutcomeFormedConsumerName 是本消费者在 inbox 键上的稳定名。同一封终局信封还投给 visibility-exception 的投影
// （扇出），两路各记各的 inbox 账。
const finalOutcomeFormedConsumerName = "settlement-accounting/request-sell-evaluation-on-final-outcome"

// FinalOutcomeFormedEventType 是本消费者认的事件类型：parcel-shipment 落下一份终局采用判断（终局或不采用、首派生或
// 重派生）后交出的信封。字符串由消费方自己写出，两串是否相等由 cmd/parcel-dispatch 的真库装配用例钉。
const FinalOutcomeFormedEventType eventing.EventType = "parcel-shipment.final-outcome.formed"

// FormedFinalOutcome 是译码后的终局采用键引用，恰是提供方 FindByKey 的四维。是不是已终局的「有效交付」、生效时点在哪
// 一刻，都按键回读终局记录再判，信封上没有也不该有。
type FormedFinalOutcome struct {
	TenantID string
	Parcel   string
	Kind     string
	Version  string
}

// FormedFinalOutcomeHandler 是本消费者转交的处理方。真实装配接 adapters/parcelshipment 的 RequestOnFinalOutcomeAdapter。
type FormedFinalOutcomeHandler interface {
	HandleFormedFinalOutcome(ctx context.Context, formed FormedFinalOutcome) error
}

// FinalOutcomeFormedConsumer 把 PS 终局采用信封推进消费门（ADR-0190 决定二「有效交付已终局」）。
type FinalOutcomeFormedConsumer struct {
	gate *inboxconsume.Gate[FormedFinalOutcome]
}

func NewFinalOutcomeFormedConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	handler FormedFinalOutcomeHandler,
) (*FinalOutcomeFormedConsumer, error) {
	if transactor == nil {
		return nil, fmt.Errorf("settlement accounting inbox: transactor is nil")
	}
	if store == nil {
		return nil, fmt.Errorf("settlement accounting inbox: inbox store is nil")
	}
	if handler == nil {
		return nil, fmt.Errorf("settlement accounting inbox: handler is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[FormedFinalOutcome]{
		Transactor:     transactor,
		Store:          store,
		Name:           finalOutcomeFormedConsumerName,
		EventType:      FinalOutcomeFormedEventType,
		Decode:         decodeFormedFinalOutcome,
		Handle:         handler.HandleFormedFinalOutcome,
		UnexpectedType: "settlement accounting inbox",
		HandleVerb:     "handle formed final outcome",
	})
	if err != nil {
		return nil, err
	}
	return &FinalOutcomeFormedConsumer{gate: gate}, nil
}

func (consumer *FinalOutcomeFormedConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

// decodeFormedFinalOutcome 译载荷。四维缺一即毒丸，理由同收寄采用那一路；版本尤其不能缺——重派生走新版本，按三维
// 读「当前版」会把更正与原判断叠成一次查找。
func decodeFormedFinalOutcome(payload []byte) (FormedFinalOutcome, error) {
	var body struct {
		TenantID string `json:"tenantId"`
		Parcel   string `json:"parcel"`
		Kind     string `json:"kind"`
		Version  string `json:"version"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return FormedFinalOutcome{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Parcel == "" || body.Kind == "" || body.Version == "" {
		return FormedFinalOutcome{}, fmt.Errorf("%w: missing final adoption key fields", ErrPoisonEnvelope)
	}
	return FormedFinalOutcome{
		TenantID: body.TenantID,
		Parcel:   body.Parcel,
		Kind:     body.Kind,
		Version:  body.Version,
	}, nil
}
